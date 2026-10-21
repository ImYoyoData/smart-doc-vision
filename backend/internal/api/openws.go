package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/service"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type wsSession struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	closed bool
}

func (sess *wsSession) write(v any) error {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		return fmt.Errorf("connection closed")
	}
	_ = sess.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return sess.conn.WriteJSON(v)
}

func (sess *wsSession) close() {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		return
	}
	sess.closed = true
	_ = sess.conn.Close()
}

type wsFrame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (s *Server) openWS(c *gin.Context) {
	project, audit, trigger, aerr := s.authenticateWS(c)
	if aerr != nil {
		fail(c, aerr.Status, aerr.Code, aerr.Message, nil)
		return
	}
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	sess := &wsSession{conn: conn}
	defer sess.close()
	conn.SetReadLimit(10 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	_ = sess.write(gin.H{
		"type": "ready",
		"data": gin.H{"project": project.Code, "audit": audit.Code, "message": "认证成功，可发送 submit 帧"},
	})

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				sess.mu.Lock()
				if sess.closed {
					sess.mu.Unlock()
					return
				}
				_ = sess.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := sess.conn.WriteMessage(websocket.PingMessage, nil)
				sess.mu.Unlock()
				if err != nil {
					sess.close()
					return
				}
			}
		}
	}()
	defer close(done)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		var frame wsFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			_ = sess.write(gin.H{"type": "error", "code": "INVALID_FRAME", "message": "帧必须是合法 JSON"})
			continue
		}
		switch frame.Type {
		case "ping":
			_ = sess.write(gin.H{"type": "pong", "data": gin.H{"ts": time.Now().UnixMilli()}})
		case "submit":
			var input model.TaskInput
			if err := json.Unmarshal(frame.Data, &input); err != nil {
				_ = sess.write(gin.H{"type": "error", "code": "INVALID_DATA", "message": "data 必须是任务对象"})
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			outcome, err := s.tasks.Submit(ctx, service.SubmitInput{
				ProjectID: project.ID, AuditID: audit.ID, SourceType: model.TriggerWebSocket,
				SourceID: trigger.ID, Input: input,
			})
			cancel()
			if err != nil {
				code, message := "SUBMIT_FAILED", err.Error()
				if se, okSE := err.(*service.SubmitError); okSE {
					code, message = se.Code, se.Message
				}
				_ = sess.write(gin.H{"type": "error", "code": code, "message": message})
				continue
			}
			_ = sess.write(gin.H{
				"type": "ack",
				"data": gin.H{
					"task_id": outcome.Task.ID, "task_no": outcome.Task.TaskNo,
					"status": outcome.Task.Status, "deduplicated": outcome.Duplicated,
				},
			})
			go s.pushWSResult(sess, outcome.Task.ID)
		default:
			_ = sess.write(gin.H{"type": "error", "code": "UNSUPPORTED_FRAME", "message": "支持 submit / ping 帧"})
		}
	}
}

func (s *Server) pushWSResult(sess *wsSession, taskID uint64) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		var task model.Task
		if err := s.db.First(&task, taskID).Error; err != nil {
			return
		}
		if !isAutoTerminal(task.Status) {
			continue
		}
		if err := sess.write(gin.H{"type": "result", "data": openTaskView(task, true)}); err != nil {
			return
		}
		return
	}
}

func (s *Server) authenticateWS(c *gin.Context) (*model.Project, *model.Audit, *model.TriggerSource, *openAuthError) {
	appKey := strings.TrimSpace(c.Query("app_key"))
	timestamp := strings.TrimSpace(c.Query("timestamp"))
	nonce := strings.TrimSpace(c.Query("nonce"))
	signature := strings.TrimSpace(c.Query("sign"))
	if appKey == "" || timestamp == "" || nonce == "" || signature == "" {
		return nil, nil, nil, &openAuthError{http.StatusUnauthorized, "UNAUTHORIZED", "缺少 app_key / timestamp / nonce / sign"}
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || deltaTooLarge(time.UnixMilli(ts)) {
		return nil, nil, nil, &openAuthError{http.StatusBadRequest, "TIMESTAMP_EXPIRED", "时间戳无效或偏差超过 5 分钟"}
	}
	if len(nonce) < 8 || len(nonce) > 128 {
		return nil, nil, nil, &openAuthError{http.StatusBadRequest, "INVALID_PARAM", "Nonce 长度需在 8~128 之间"}
	}
	var credential model.APICredential
	if err := s.db.Where("app_key = ?", appKey).First(&credential).Error; err != nil {
		return nil, nil, nil, &openAuthError{http.StatusUnauthorized, "UNAUTHORIZED", "AppKey 无效"}
	}
	if credential.Status != model.CredentialActive {
		return nil, nil, nil, &openAuthError{http.StatusUnauthorized, "CREDENTIAL_DISABLED", "AppKey 已停用"}
	}
	var project model.Project
	if err := s.db.First(&project, credential.ProjectID).Error; err != nil || project.DeletedAt != nil {
		return nil, nil, nil, &openAuthError{http.StatusForbidden, "PROJECT_DELETED", "项目不存在或已删除"}
	}
	if c.Param("projectCode") != project.Code {
		return nil, nil, nil, &openAuthError{http.StatusForbidden, "PROJECT_OUT_OF_SCOPE", "凭证不属于该路径指定的项目"}
	}
	secret, err := s.cipher.Decrypt(credential.SecretEnc)
	if err != nil || secret == "" {
		return nil, nil, nil, &openAuthError{http.StatusServiceUnavailable, "CREDENTIAL_UNAVAILABLE", "凭证密钥不可用"}
	}
	path := c.Request.URL.EscapedPath()
	canonical := "GET\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + security.BodySHA256(nil)
	expected := security.SignHMAC(secret, canonical)
	if !security.SignatureEqual(expected, strings.ToLower(signature)) {
		return nil, nil, nil, &openAuthError{http.StatusBadRequest, "INVALID_SIGNATURE", "WebSocket 签名校验失败"}
	}
	if s.redis != nil {
		replayKey := fmt.Sprintf("sdv:open:nonce:%s:%s", credential.AppKey, nonce)
		okReplay, replayErr := s.redis.SetNX(c.Request.Context(), replayKey, "1", 10*time.Minute).Result()
		if replayErr != nil {
			return nil, nil, nil, &openAuthError{http.StatusServiceUnavailable, "DOWNSTREAM_UNAVAILABLE", "防重放服务暂不可用"}
		}
		if !okReplay {
			return nil, nil, nil, &openAuthError{http.StatusBadRequest, "REPLAY_ATTACK", "Nonce 已被使用"}
		}
	}
	now := time.Now()
	_ = s.db.Model(&model.APICredential{}).Where("id = ?", credential.ID).Update("last_used_at", now).Error
	credential.LastUsedAt = &now
	var audit model.Audit
	if err := s.db.Where("project_id = ? AND code = ? AND deleted_at IS NULL", project.ID, c.Param("auditCode")).First(&audit).Error; err != nil {
		return nil, nil, nil, &openAuthError{http.StatusNotFound, "NOT_FOUND", "审核配置不存在"}
	}
	if audit.Status != model.AuditActive {
		return nil, nil, nil, &openAuthError{http.StatusForbidden, "AUDIT_NOT_ACTIVE", "审核配置未启用"}
	}
	if !credentialAllowsAudit(credential, audit.ID) {
		return nil, nil, nil, &openAuthError{http.StatusForbidden, "AUDIT_OUT_OF_SCOPE", "该凭证未被授权访问此审核配置"}
	}
	var trigger model.TriggerSource
	if err := s.db.Where("audit_id = ? AND type = ?", audit.ID, model.TriggerWebSocket).First(&trigger).Error; err != nil || trigger.Status != "ACTIVE" {
		return nil, nil, nil, &openAuthError{http.StatusForbidden, "TRIGGER_DISABLED", "该审核配置未启用 WebSocket 触发"}
	}
	return &project, &audit, &trigger, nil
}

func deltaTooLarge(t time.Time) bool {
	delta := time.Since(t)
	return delta > 5*time.Minute || delta < -5*time.Minute
}
