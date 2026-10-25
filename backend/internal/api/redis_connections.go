package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
	triggerpkg "smartdoc-vision/backend/internal/trigger"
)

type redisConnectionInput struct {
	Name               string  `json:"name"`
	Addr               string  `json:"addr"`
	Username           *string `json:"username"`
	Password           string  `json:"password"`
	TLS                *bool   `json:"tls"`
	Status             string  `json:"status"`
	AutoResumeTriggers *bool   `json:"auto_resume_triggers"`
}

type redisConnectionView struct {
	model.RedisConnection
	HasPassword bool `json:"has_password"`
}

func (s *Server) listRedisConnections(c *gin.Context) {
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") && !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权查看 Redis 连接")
		return
	}
	var connections []model.RedisConnection
	if err := s.db.Where("project_id = ?", project.ID).Order("id asc").Find(&connections).Error; err != nil {
		serverError(c, err)
		return
	}
	items := make([]redisConnectionView, 0, len(connections))
	for _, connection := range connections {
		items = append(items, redisConnectionView{
			RedisConnection: connection,
			HasPassword:     connection.PasswordEnc != "",
		})
	}
	ok(c, gin.H{"items": items, "total": len(items)})
}

func (s *Server) createRedisConnection(c *gin.Context) {
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权配置 Redis 连接")
		return
	}
	var req redisConnectionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	connection, err := s.buildRedisConnection(project.ID, req, nil)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.db.Create(&connection).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			conflict(c, "NAME_CONFLICT", "当前项目下已存在同名 Redis 连接")
			return
		}
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "REDIS_CONNECTION_CREATE", "REDIS_CONNECTION", fmt.Sprint(connection.ID), "SUCCESS",
		map[string]any{"name": connection.Name, "addr": connection.Addr})
	created(c, connection)
}

func (s *Server) updateRedisConnection(c *gin.Context) {
	connection, err := s.findRedisConnection(c)
	if err != nil {
		return
	}
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权配置 Redis 连接")
		return
	}
	var req redisConnectionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	previousAddr := connection.Addr
	previousUser := connection.Username
	previousPassword := connection.PasswordEnc
	previousTLS := connection.TLS
	previousStatus := connection.Status
	if err := s.applyRedisConnectionInput(connection, req); err != nil {
		badRequest(c, err.Error())
		return
	}
	configChanged := connection.Addr != previousAddr || connection.Username != previousUser ||
		connection.PasswordEnc != previousPassword || connection.TLS != previousTLS
	statusChanged := connection.Status != previousStatus
	if configChanged || statusChanged {
		connection.Health = model.RedisHealthUnknown
		connection.LastTestOK = false
	}
	if err := s.db.Save(connection).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			conflict(c, "NAME_CONFLICT", "当前项目下已存在同名 Redis 连接")
			return
		}
		serverError(c, err)
		return
	}
	if statusChanged && connection.Status == model.RedisConnectionDisabled {
		_ = triggerpkg.PauseTriggersForConnection(s.db, connection.ID, model.TriggerPauseConnectionDisabled)
	}
	if (configChanged || statusChanged) && connection.Status == model.RedisConnectionActive {
		reason := model.TriggerPauseConnectionUnhealthy
		_ = triggerpkg.PauseTriggersForConnection(s.db, connection.ID, reason)
	}
	s.writeAudit(c, &connection.ProjectID, "REDIS_CONNECTION_UPDATE", "REDIS_CONNECTION", fmt.Sprint(connection.ID), "SUCCESS",
		map[string]any{"name": connection.Name, "status": connection.Status, "config_changed": configChanged})
	ok(c, connection)
}

func (s *Server) deleteRedisConnection(c *gin.Context) {
	connection, err := s.findRedisConnection(c)
	if err != nil {
		return
	}
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权删除 Redis 连接")
		return
	}
	var triggerCount int64
	if err := s.db.Model(&model.TriggerSource{}).Where("redis_connection_id = ?", connection.ID).Count(&triggerCount).Error; err != nil {
		serverError(c, err)
		return
	}
	if triggerCount > 0 {
		conflict(c, "REDIS_CONNECTION_IN_USE", fmt.Sprintf("该 Redis 连接仍被 %d 个触发源引用，请先解绑", triggerCount))
		return
	}
	if used, err := s.redisConnectionUsedByResultAction(connection.ProjectID, connection.ID); err != nil {
		serverError(c, err)
		return
	} else if used {
		conflict(c, "REDIS_CONNECTION_IN_USE", "该 Redis 连接仍被结果动作引用，请先调整审核配置")
		return
	}
	if err := s.db.Delete(connection).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &connection.ProjectID, "REDIS_CONNECTION_DELETE", "REDIS_CONNECTION", fmt.Sprint(connection.ID), "SUCCESS",
		map[string]any{"name": connection.Name})
	ok(c, gin.H{"deleted": true})
}

func (s *Server) testRedisConnection(c *gin.Context) {
	connection, err := s.findRedisConnection(c)
	if err != nil {
		return
	}
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权测试 Redis 连接")
		return
	}
	password := ""
	if connection.PasswordEnc != "" {
		password, err = s.cipher.Decrypt(connection.PasswordEnc)
		if err != nil {
			serverError(c, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	latency, testErr := triggerpkg.TestRedisConnection(ctx, *connection, password)
	now := time.Now()
	updates := map[string]any{
		"last_test_at":  now,
		"last_test_ok":  testErr == nil,
		"last_test_ms":  latency.Milliseconds(),
		"last_test_msg": "连接成功",
		"health":        model.RedisHealthOK,
	}
	if testErr != nil {
		updates["last_test_msg"] = truncateText(testErr.Error(), 500)
		updates["health"] = model.RedisHealthUnhealthy
	}
	if updateErr := s.db.Model(&model.RedisConnection{}).Where("id = ?", connection.ID).Updates(updates).Error; updateErr != nil {
		serverError(c, updateErr)
		return
	}
	if testErr != nil {
		reason := model.TriggerPauseConnectionUnhealthy
		if connection.Status == model.RedisConnectionDisabled {
			reason = model.TriggerPauseConnectionDisabled
		}
		_ = triggerpkg.PauseTriggersForConnection(s.db, connection.ID, reason)
		s.writeAudit(c, &connection.ProjectID, "REDIS_CONNECTION_TEST", "REDIS_CONNECTION", fmt.Sprint(connection.ID), "FAILED",
			map[string]any{"error": truncateText(testErr.Error(), 200)})
		fail(c, http.StatusUnprocessableEntity, "REDIS_CONNECTION_FAILED", testErr.Error(), gin.H{
			"ok": false, "latency_ms": latency.Milliseconds(), "health": model.RedisHealthUnhealthy,
		})
		return
	}
	if connection.Status == model.RedisConnectionActive && connection.AutoResumeTriggers {
		_ = triggerpkg.ResumeTriggersForConnection(s.db, connection.ID, model.TriggerPauseConnectionUnhealthy)
	}
	s.writeAudit(c, &connection.ProjectID, "REDIS_CONNECTION_TEST", "REDIS_CONNECTION", fmt.Sprint(connection.ID), "SUCCESS",
		map[string]any{"latency_ms": latency.Milliseconds()})
	ok(c, gin.H{
		"ok": true, "latency_ms": latency.Milliseconds(),
		"health": model.RedisHealthOK, "message": "连接成功",
	})
}

func (s *Server) findRedisConnection(c *gin.Context) (*model.RedisConnection, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "Redis 连接 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var connection model.RedisConnection
	if err := s.db.First(&connection, id).Error; err != nil {
		notFound(c, "Redis 连接不存在")
		return nil, err
	}
	if !s.requireProjectAccess(c, connection.ProjectID) {
		return nil, fmt.Errorf("redis connection out of scope")
	}
	return &connection, nil
}

func (s *Server) buildRedisConnection(projectID uint64, req redisConnectionInput, existing *model.RedisConnection) (model.RedisConnection, error) {
	connection := model.RedisConnection{ProjectID: projectID, Status: model.RedisConnectionDisabled, Health: model.RedisHealthUnknown, AutoResumeTriggers: true}
	if existing != nil {
		connection = *existing
	}
	if err := s.applyRedisConnectionInput(&connection, req); err != nil {
		return connection, err
	}
	return connection, nil
}

func (s *Server) applyRedisConnectionInput(connection *model.RedisConnection, req redisConnectionInput) error {
	if name := strings.TrimSpace(req.Name); name != "" {
		if len([]rune(name)) < 2 || len([]rune(name)) > 64 {
			return fmt.Errorf("连接名称需为 2~64 个字符")
		}
		connection.Name = name
	}
	if connection.Name == "" {
		return fmt.Errorf("请填写连接名称")
	}
	addr := strings.TrimSpace(req.Addr)
	if addr != "" {
		if !strings.Contains(addr, ":") {
			addr = net.JoinHostPort(addr, "6379")
		}
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("Redis 地址需为 host:port，例如 127.0.0.1:6379")
		}
		connection.Addr = addr
	}
	if connection.Addr == "" {
		return fmt.Errorf("请填写 Redis 地址")
	}
	if req.Username != nil {
		connection.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != "" {
		encrypted, err := s.cipher.Encrypt(req.Password)
		if err != nil {
			return fmt.Errorf("密钥加密失败")
		}
		connection.PasswordEnc = encrypted
	}
	if req.TLS != nil {
		connection.TLS = *req.TLS
	}
	if req.AutoResumeTriggers != nil {
		connection.AutoResumeTriggers = *req.AutoResumeTriggers
	}
	if req.Status != "" {
		if !oneOf(req.Status, model.RedisConnectionActive, model.RedisConnectionDisabled) {
			return fmt.Errorf("状态需为 ACTIVE 或 DISABLED")
		}
		connection.Status = req.Status
	}
	return nil
}

func (s *Server) redisConnectionUsedByResultAction(projectID, connectionID uint64) (bool, error) {
	var audits []model.Audit
	if err := s.db.Select("id", "routes").Where("project_id = ? AND deleted_at IS NULL", projectID).Find(&audits).Error; err != nil {
		return false, err
	}
	for _, audit := range audits {
		var routes map[string]struct {
			Actions []map[string]any `json:"actions"`
		}
		if err := json.Unmarshal(audit.Routes, &routes); err != nil {
			continue
		}
		for _, route := range routes {
			for _, action := range route.Actions {
				if !strings.EqualFold(fmt.Sprint(action["type"]), "redis") {
					continue
				}
				switch value := action["connection_id"].(type) {
				case float64:
					if uint64(value) == connectionID {
						return true, nil
					}
				case json.Number:
					if number, err := value.Int64(); err == nil && uint64(number) == connectionID {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}
