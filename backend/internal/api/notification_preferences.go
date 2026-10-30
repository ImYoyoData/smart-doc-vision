package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

type notificationPreferenceView struct {
	EmailNotifyEnabled bool   `json:"email_notify_enabled"`
	HasEmail           bool   `json:"has_email"`
	Email              string `json:"email"`
	HasChannel         bool   `json:"has_channel"`
}

func (s *Server) getNotificationPreference(c *gin.Context) {
	user := currentUser(c)
	projectID := uint64(0)
	if value := c.Query("project_id"); value != "" {
		id, okID := parseUint(value)
		if !okID {
			badRequest(c, "project_id 无效")
			return
		}
		projectID = id
	}
	hasChannel := false
	if projectID > 0 {
		if !s.requireProjectAccess(c, projectID) {
			return
		}
		hasChannel = s.hasActiveEmailChannel(projectID)
	}
	ok(c, notificationPreferenceView{
		EmailNotifyEnabled: user.EmailNotifyEnabled,
		HasEmail:           strings.TrimSpace(user.Email) != "",
		Email:              user.Email,
		HasChannel:         hasChannel,
	})
}

func (s *Server) updateNotificationPreference(c *gin.Context) {
	user := currentUser(c)
	var req struct {
		EmailNotifyEnabled bool   `json:"email_notify_enabled"`
		ProjectID          uint64 `json:"project_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if req.EmailNotifyEnabled {
		if strings.TrimSpace(user.Email) == "" {
			badRequest(c, "请先在个人中心配置邮箱地址")
			return
		}
		if req.ProjectID == 0 {
			badRequest(c, "请选择项目")
			return
		}
		if !s.requireProjectAccess(c, req.ProjectID) {
			return
		}
		if !s.hasActiveEmailChannel(req.ProjectID) {
			badRequest(c, "当前项目没有可用且测试通过的邮箱通知渠道，请联系管理员配置")
			return
		}
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).
		Update("email_notify_enabled", req.EmailNotifyEnabled).Error; err != nil {
		serverError(c, err)
		return
	}
	user.EmailNotifyEnabled = req.EmailNotifyEnabled
	s.writeAudit(c, nil, "PROFILE_NOTIFICATION_UPDATE", "USER", fmt.Sprint(user.ID), "SUCCESS",
		map[string]any{"email_notify_enabled": req.EmailNotifyEnabled})
	ok(c, notificationPreferenceView{
		EmailNotifyEnabled: user.EmailNotifyEnabled,
		HasEmail:           strings.TrimSpace(user.Email) != "",
		Email:              user.Email,
		HasChannel:         req.ProjectID > 0 && s.hasActiveEmailChannel(req.ProjectID),
	})
}

func (s *Server) hasActiveEmailChannel(projectID uint64) bool {
	var count int64
	_ = s.db.Model(&model.NotificationChannel{}).
		Where("project_id = ? AND type = ? AND status = ? AND last_test_ok = ?",
			projectID, model.NotificationTypeEmail, model.NotificationStatusActive, true).
		Count(&count).Error
	return count > 0
}
