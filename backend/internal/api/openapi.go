package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
	"smartdoc-vision/backend/internal/service"
)

type openAuthError struct {
	Status  int
	Code    string
	Message string
}

func (e *openAuthError) Error() string { return e.Message }

type openAuthContext struct {
	Project    model.Project
	Credential model.APICredential
	Secret     string
}

type openSubmitRequest struct {
	BizKey      string            `json:"biz_key"`
	Media       []model.MediaItem `json:"media"`
	Ext         map[string]any    `json:"ext"`
	CallbackURL string            `json:"callback_url"`
	Mode        string            `json:"mode"`
	WaitMS      int               `json:"wait_ms"`
}

func (s *Server) openSubmit(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 20<<20))
	if err != nil {
		fail(c, http.StatusBadRequest, "INVALID_PARAM", "读取请求体失败", nil)
		return
	}
	auth, aerr := s.authenticateOpen(c, body)
	if aerr != nil {
		fail(c, aerr.Status, aerr.Code, aerr.Message, nil)
		return
	}
	project := auth.Project
	auditCode := strings.TrimSpace(c.Param("auditCode"))
	var audit model.Audit
	if err := s.db.Where("project_id = ? AND code = ? AND deleted_at IS NULL", project.ID, auditCode).First(&audit).Error; err != nil {
		fail(c, http.StatusNotFound, "NOT_FOUND", "审核配置不存在", nil)
		return
	}
	if audit.Status != model.AuditActive {
		fail(c, http.StatusForbidden, "AUDIT_NOT_ACTIVE", "审核配置未启用", nil)
		return
	}
	if !credentialAllowsAudit(auth.Credential, audit.ID) {
		fail(c, http.StatusForbidden, "AUDIT_OUT_OF_SCOPE", "该凭证未被授权访问此审核配置", nil)
		return
	}
	var trigger model.TriggerSource
	if err := s.db.Where("audit_id = ? AND type = ?", audit.ID, model.TriggerOpenAPI).First(&trigger).Error; err != nil || trigger.Status != "ACTIVE" {
		fail(c, http.StatusForbidden, "TRIGGER_DISABLED", "该审核配置未启用开放 API 触发", nil)
		return
	}
	if project.OpenAPIRateLimit > 0 && s.redis != nil {
		window := time.Now().Unix() / 60
		key := fmt.Sprintf("sdv:rate:openapi:%d:%d", project.ID, window)
		count, rateErr := s.redis.Incr(c.Request.Context(), key).Result()
		if rateErr == nil {
			if count == 1 {
				_ = s.redis.Expire(c.Request.Context(), key, 70*time.Second).Err()
			}
			if count > int64(project.OpenAPIRateLimit) {
				c.Header("Retry-After", "60")
				fail(c, http.StatusTooManyRequests, "RATE_LIMITED",
					fmt.Sprintf("开放 API 调用超过项目限制（%d 次/分钟）", project.OpenAPIRateLimit), nil)
				return
			}
		}
	}
	var req openSubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		fail(c, http.StatusBadRequest, "INVALID_PARAM", "请求体必须是合法 JSON", nil)
		return
	}
	input := model.TaskInput{
		BizKey:      strings.TrimSpace(req.BizKey),
		Media:       req.Media,
		Ext:         req.Ext,
		CallbackURL: strings.TrimSpace(req.CallbackURL),
	}
	outcome, err := s.tasks.Submit(c.Request.Context(), service.SubmitInput{
		ProjectID: project.ID, AuditID: audit.ID, SourceType: model.TriggerOpenAPI,
		SourceID: trigger.ID, Input: input,
	})
	if err != nil {
		var se *service.SubmitError
		if errors.As(err, &se) {
			fail(c, se.Status, se.Code, se.Message, se.Data)
			return
		}
		serverError(c, err)
		return
	}
	task := outcome.Task
	if strings.EqualFold(req.Mode, "sync") {
		waitMS := req.WaitMS
		if waitMS <= 0 {
			waitMS = 3000
		}
		if waitMS > 10000 {
			waitMS = 10000
		}
		deadline := time.Now().Add(time.Duration(waitMS) * time.Millisecond)
		for time.Now().Before(deadline) {
			var latest model.Task
			if s.db.First(&latest, task.ID).Error == nil {
				task = latest
				if isAutoTerminal(task.Status) {
					break
				}
			}
			select {
			case <-time.After(250 * time.Millisecond):
			case <-c.Request.Context().Done():
				goto done
			}
		}
	}
done:
	s.createAuditLogNoReq(&project.ID, "openapi:"+auth.Credential.AppKey, "OPENAPI_SUBMIT", "TASK", task.TaskNo, "SUCCESS",
		map[string]any{"audit_id": audit.ID, "source_type": model.TriggerOpenAPI, "duplicated": outcome.Duplicated},
		c.GetString("trace_id"))
	ok(c, gin.H{
		"task_id": task.ID, "task_no": task.TaskNo, "status": task.Status, "result": task.Result,
		"deduplicated": outcome.Duplicated, "sync": strings.EqualFold(req.Mode, "sync"),
		"task": openTaskView(task, false),
	})
}

func (s *Server) openTask(c *gin.Context) {
	body, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	auth, aerr := s.authenticateOpen(c, body)
	if aerr != nil {
		fail(c, aerr.Status, aerr.Code, aerr.Message, nil)
		return
	}
	taskNo := strings.TrimSpace(c.Param("taskNo"))
	var task model.Task
	if err := s.db.Where("project_id = ? AND task_no = ?", auth.Project.ID, taskNo).First(&task).Error; err != nil {
		fail(c, http.StatusNotFound, "NOT_FOUND", "任务不存在", nil)
		return
	}
	if !credentialAllowsAudit(auth.Credential, task.AuditID) {
		fail(c, http.StatusNotFound, "NOT_FOUND", "任务不存在", nil)
		return
	}
	var audit model.Audit
	if err := s.db.Where("id = ? AND project_id = ? AND deleted_at IS NULL", task.AuditID, task.ProjectID).First(&audit).Error; err != nil {
		fail(c, http.StatusNotFound, "NOT_FOUND", "任务不存在", nil)
		return
	}
	ok(c, openTaskView(task, true))
}

func openTaskView(task model.Task, includeInput bool) gin.H {
	view := gin.H{
		"task_id": task.ID, "task_no": task.TaskNo, "project_id": task.ProjectID,
		"audit_id": task.AuditID, "biz_key": task.BizKey, "source_type": task.SourceType,
		"media_type": task.MediaType, "status": task.Status, "result": task.Result,
		"result_detail": task.ResultJSON, "error_message": task.ErrorMessage,
		"duration_ms": task.DurationMS, "retry_count": task.RetryCount,
		"created_at": task.CreatedAt, "finished_at": task.FinishedAt,
		"reviewed_at": task.ReviewedAt,
	}
	if includeInput {
		view["input"] = task.InputJSON
		view["model_result"] = task.ModelResult
		view["rule_trace"] = task.RuleTrace
	}
	return view
}

func isAutoTerminal(status string) bool {
	switch status {
	case model.TaskSuccess, model.TaskReviewPending, model.TaskFailed, model.TaskStopped:
		return true
	default:
		return false
	}
}

func (s *Server) authenticateOpen(c *gin.Context, body []byte) (*openAuthContext, *openAuthError) {
	appKey := strings.TrimSpace(c.GetHeader("X-Api-Key"))
	timestamp := strings.TrimSpace(c.GetHeader("X-Timestamp"))
	nonce := strings.TrimSpace(c.GetHeader("X-Nonce"))
	signature := strings.TrimSpace(c.GetHeader("X-Signature"))
	if appKey == "" || timestamp == "" || nonce == "" || signature == "" {
		return nil, &openAuthError{http.StatusUnauthorized, "UNAUTHORIZED", "缺少 X-Api-Key / X-Timestamp / X-Nonce / X-Signature"}
	}
	if len(nonce) < 8 || len(nonce) > 128 {
		return nil, &openAuthError{http.StatusBadRequest, "INVALID_PARAM", "Nonce 长度需在 8~128 之间"}
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, &openAuthError{http.StatusBadRequest, "INVALID_PARAM", "X-Timestamp 必须是毫秒时间戳"}
	}
	if delta := time.Since(time.UnixMilli(ts)); delta > 5*time.Minute || delta < -5*time.Minute {
		return nil, &openAuthError{http.StatusBadRequest, "TIMESTAMP_EXPIRED", "请求时间与服务器偏差超过 5 分钟"}
	}
	var credential model.APICredential
	if err := s.db.Where("app_key = ?", appKey).First(&credential).Error; err != nil {
		return nil, &openAuthError{http.StatusUnauthorized, "UNAUTHORIZED", "AppKey 无效"}
	}
	if credential.Status != model.CredentialActive {
		return nil, &openAuthError{http.StatusUnauthorized, "CREDENTIAL_DISABLED", "AppKey 已停用"}
	}
	var project model.Project
	if err := s.db.First(&project, credential.ProjectID).Error; err != nil || project.DeletedAt != nil {
		return nil, &openAuthError{http.StatusForbidden, "PROJECT_DELETED", "项目不存在或已删除"}
	}
	projectCode := strings.TrimSpace(c.Param("projectCode"))
	if projectCode == "" || project.Code != projectCode {
		return nil, &openAuthError{http.StatusForbidden, "PROJECT_OUT_OF_SCOPE", "凭证不属于该路径指定的项目"}
	}
	secret, err := s.cipher.Decrypt(credential.SecretEnc)
	if err != nil || secret == "" {
		return nil, &openAuthError{http.StatusServiceUnavailable, "CREDENTIAL_UNAVAILABLE", "凭证密钥不可用，请管理员轮换"}
	}
	path := c.Request.URL.EscapedPath()
	canonical := c.Request.Method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + security.BodySHA256(body)
	expected := security.SignHMAC(secret, canonical)
	if !security.SignatureEqual(expected, strings.ToLower(signature)) {
		return nil, &openAuthError{http.StatusBadRequest, "INVALID_SIGNATURE", "签名校验失败"}
	}
	if s.redis != nil {
		replayKey := fmt.Sprintf("sdv:open:nonce:%s:%s", credential.AppKey, nonce)
		okReplay, replayErr := s.redis.SetNX(c.Request.Context(), replayKey, "1", 10*time.Minute).Result()
		if replayErr != nil {
			return nil, &openAuthError{http.StatusServiceUnavailable, "DOWNSTREAM_UNAVAILABLE", "防重放服务暂不可用"}
		}
		if !okReplay {
			return nil, &openAuthError{http.StatusBadRequest, "REPLAY_ATTACK", "Nonce 已被使用"}
		}
	}
	now := time.Now()
	_ = s.db.Model(&model.APICredential{}).Where("id = ?", credential.ID).Update("last_used_at", now).Error
	credential.LastUsedAt = &now
	return &openAuthContext{Project: project, Credential: credential, Secret: secret}, nil
}

func (s *Server) reviewSummary(c *gin.Context) {
	pageSize := 5
	user := currentUser(c)
	q := s.db.Model(&model.Task{}).Where("status = ?", model.TaskReviewPending)
	projectID := uint64(0)
	if pid := c.Query("project_id"); pid != "" {
		if id, okID := parseUint(pid); okID {
			if !s.requireProjectAccess(c, id) {
				return
			}
			projectID = id
			q = q.Where("project_id = ?", id)
		}
	}
	if projectID == 0 {
		if !isSuperAdmin(user) {
			forbidden(c, "非超级管理员查看复核队列时必须选择项目")
			return
		}
	}
	var total int64
	q.Count(&total)
	var recent []model.Task
	_ = q.Select("id, task_no, project_id, audit_id, source_type, biz_key, media_type, status, result, provider_id, error_message, retry_count, duration_ms, reviewer_id, reviewed_at, started_at, finished_at, created_at, updated_at").
		Order("created_at asc").Limit(pageSize).Find(&recent).Error
	s.populateTaskNames(recent)
	var oldest *time.Time
	if total > 0 {
		var oldestValue time.Time
		_ = q.Order("created_at asc").Limit(1).Pluck("created_at", &oldestValue).Error
		if !oldestValue.IsZero() {
			oldest = &oldestValue
		}
	}
	type auditCount struct {
		AuditID uint64 `json:"audit_id"`
		Count   int64  `json:"count"`
	}
	var counts []auditCount
	_ = s.db.Model(&model.Task{}).
		Select("audit_id, COUNT(*) AS count").
		Where("status = ?", model.TaskReviewPending).
		Group("audit_id").Order("count desc").Scan(&counts).Error
	byAudit := make([]gin.H, 0, len(counts))
	for _, c := range counts {
		var audit model.Audit
		_ = s.db.First(&audit, c.AuditID).Error
		byAudit = append(byAudit, gin.H{"audit_id": c.AuditID, "audit_name": audit.Name, "count": c.Count})
	}
	ok(c, gin.H{
		"pending": total, "oldest_at": oldest, "recent": recent, "page_size": pageSize,
		"by_audit": byAudit,
	})
}
