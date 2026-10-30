package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/notify"
)

type notificationEmailConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	Security    string `json:"security"`
	TimeoutSec  int    `json:"timeout_sec"`
}

type notificationChannelInput struct {
	Name     string                  `json:"name"`
	Type     string                  `json:"type"`
	Provider string                  `json:"provider"`
	Status   string                  `json:"status"`
	Config   notificationEmailConfig `json:"config"`
}

type notificationChannelView struct {
	ID              uint64                  `json:"id"`
	ProjectID       uint64                  `json:"project_id"`
	Name            string                  `json:"name"`
	Type            string                  `json:"type"`
	Provider        string                  `json:"provider"`
	Status          string                  `json:"status"`
	Config          notificationEmailConfig `json:"config"`
	HasPassword     bool                    `json:"has_password"`
	LastTestAt      *time.Time              `json:"last_test_at"`
	LastTestOK      bool                    `json:"last_test_ok"`
	LastTestMessage string                  `json:"last_test_message"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

func (s *Server) listNotificationChannels(c *gin.Context) {
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	if !s.hasPermission(currentUser(c).Role, "notification:write") {
		forbidden(c, "当前角色无权查看通知渠道")
		return
	}
	var channels []model.NotificationChannel
	if err := s.db.Where("project_id = ?", project.ID).Order("id asc").Find(&channels).Error; err != nil {
		serverError(c, err)
		return
	}
	items := make([]notificationChannelView, 0, len(channels))
	for _, channel := range channels {
		items = append(items, s.notificationChannelView(channel))
	}
	ok(c, gin.H{"items": items, "total": len(items)})
}

func (s *Server) createNotificationChannel(c *gin.Context) {
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	if !s.hasPermission(currentUser(c).Role, "notification:write") {
		forbidden(c, "当前角色无权配置通知渠道")
		return
	}
	var req notificationChannelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	channel, err := s.buildNotificationChannel(project.ID, req, nil)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.db.Create(&channel).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			conflict(c, "NAME_CONFLICT", "当前项目下已存在同名通知渠道")
			return
		}
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "NOTIFICATION_CHANNEL_CREATE", "NOTIFICATION_CHANNEL", fmt.Sprint(channel.ID), "SUCCESS",
		map[string]any{"name": channel.Name, "type": channel.Type, "provider": channel.Provider})
	created(c, s.notificationChannelView(channel))
}

func (s *Server) updateNotificationChannel(c *gin.Context) {
	channel, err := s.findNotificationChannel(c)
	if err != nil {
		return
	}
	if !s.hasPermission(currentUser(c).Role, "notification:write") {
		forbidden(c, "当前角色无权配置通知渠道")
		return
	}
	var req notificationChannelInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	updated, err := s.buildNotificationChannel(channel.ProjectID, req, channel)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	previousConfig, previousErr := s.decryptNotificationConfig(*channel)
	updatedConfig, updatedErr := s.decryptNotificationConfig(updated)
	configChanged := previousErr != nil || updatedErr != nil || previousConfig != updatedConfig
	if configChanged {
		updated.LastTestAt = nil
		updated.LastTestOK = false
		updated.LastTestMessage = ""
	}
	if err := s.db.Save(&updated).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			conflict(c, "NAME_CONFLICT", "当前项目下已存在同名通知渠道")
			return
		}
		serverError(c, err)
		return
	}
	s.writeAudit(c, &updated.ProjectID, "NOTIFICATION_CHANNEL_UPDATE", "NOTIFICATION_CHANNEL", fmt.Sprint(updated.ID), "SUCCESS",
		map[string]any{"name": updated.Name, "status": updated.Status, "config_changed": configChanged})
	ok(c, s.notificationChannelView(updated))
}

func (s *Server) updateNotificationChannelStatus(c *gin.Context) {
	channel, err := s.findNotificationChannel(c)
	if err != nil {
		return
	}
	if !s.hasPermission(currentUser(c).Role, "notification:write") {
		forbidden(c, "当前角色无权配置通知渠道")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if !oneOf(status, model.NotificationStatusActive, model.NotificationStatusDisabled) {
		badRequest(c, "渠道状态不合法")
		return
	}
	if err := s.db.Model(&model.NotificationChannel{}).Where("id = ?", channel.ID).Update("status", status).Error; err != nil {
		serverError(c, err)
		return
	}
	channel.Status = status
	s.writeAudit(c, &channel.ProjectID, "NOTIFICATION_CHANNEL_STATUS", "NOTIFICATION_CHANNEL", fmt.Sprint(channel.ID), "SUCCESS",
		map[string]any{"name": channel.Name, "status": status})
	ok(c, s.notificationChannelView(*channel))
}

func (s *Server) deleteNotificationChannel(c *gin.Context) {
	channel, err := s.findNotificationChannel(c)
	if err != nil {
		return
	}
	if !s.hasPermission(currentUser(c).Role, "notification:write") {
		forbidden(c, "当前角色无权删除通知渠道")
		return
	}
	if err := s.db.Delete(channel).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &channel.ProjectID, "NOTIFICATION_CHANNEL_DELETE", "NOTIFICATION_CHANNEL", fmt.Sprint(channel.ID), "SUCCESS",
		map[string]any{"name": channel.Name})
	ok(c, gin.H{"deleted": true})
}

func (s *Server) testNotificationChannel(c *gin.Context) {
	channel, err := s.findNotificationChannel(c)
	if err != nil {
		return
	}
	if !s.hasPermission(currentUser(c).Role, "notification:write") {
		forbidden(c, "当前角色无权测试通知渠道")
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	to, err := normalizeEmail(req.To)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if to == "" {
		badRequest(c, "请填写测试收件邮箱")
		return
	}
	cfg, err := s.decryptNotificationConfig(*channel)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "NOTIFICATION_CONFIG_INVALID", "通知渠道配置无法读取", nil)
		return
	}
	var project model.Project
	_ = s.db.First(&project, channel.ProjectID).Error
	testErr := sendNotificationTestEmail(cfg, channel.Name, project.Name, to)
	now := time.Now()
	message := "测试邮件已发送"
	if testErr != nil {
		message = truncateText(testErr.Error(), 500)
	}
	if err := s.db.Model(&model.NotificationChannel{}).Where("id = ?", channel.ID).Updates(map[string]any{
		"last_test_at":      now,
		"last_test_ok":      testErr == nil,
		"last_test_message": message,
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	channel.LastTestAt = &now
	channel.LastTestOK = testErr == nil
	channel.LastTestMessage = message
	if testErr != nil {
		s.writeAudit(c, &channel.ProjectID, "NOTIFICATION_CHANNEL_TEST", "NOTIFICATION_CHANNEL", fmt.Sprint(channel.ID), "FAILED",
			map[string]any{"to": to, "error": truncateText(testErr.Error(), 200)})
		fail(c, http.StatusUnprocessableEntity, "NOTIFICATION_TEST_FAILED", message, gin.H{"ok": false, "to": to})
		return
	}
	s.writeAudit(c, &channel.ProjectID, "NOTIFICATION_CHANNEL_TEST", "NOTIFICATION_CHANNEL", fmt.Sprint(channel.ID), "SUCCESS",
		map[string]any{"to": to})
	ok(c, gin.H{"ok": true, "to": to, "message": message})
}

func (s *Server) findNotificationChannel(c *gin.Context) (*model.NotificationChannel, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "通知渠道 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var channel model.NotificationChannel
	if err := s.db.First(&channel, id).Error; err != nil {
		notFound(c, "通知渠道不存在")
		return nil, err
	}
	if !s.requireProjectAccess(c, channel.ProjectID) {
		return nil, fmt.Errorf("notification channel out of scope")
	}
	return &channel, nil
}

func (s *Server) buildNotificationChannel(projectID uint64, req notificationChannelInput, existing *model.NotificationChannel) (model.NotificationChannel, error) {
	channel := model.NotificationChannel{
		ProjectID: projectID,
		Type:      model.NotificationTypeEmail,
		Status:    model.NotificationStatusDisabled,
	}
	if existing != nil {
		channel = *existing
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 64 {
		return channel, fmt.Errorf("渠道名称需为 1~64 个字符")
	}
	channel.Name = name

	channelType := strings.ToUpper(strings.TrimSpace(req.Type))
	if channelType == "" {
		channelType = model.NotificationTypeEmail
	}
	if channelType != model.NotificationTypeEmail {
		return channel, fmt.Errorf("当前仅支持邮箱渠道")
	}
	channel.Type = channelType

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "custom"
	}
	if len(provider) > 32 {
		return channel, fmt.Errorf("服务商标识过长")
	}
	channel.Provider = provider

	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if status == "" {
		status = model.NotificationStatusDisabled
	}
	if !oneOf(status, model.NotificationStatusActive, model.NotificationStatusDisabled) {
		return channel, fmt.Errorf("渠道状态不合法")
	}
	channel.Status = status

	cfg := req.Config
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.FromName = strings.TrimSpace(cfg.FromName)
	cfg.FromAddress = strings.TrimSpace(cfg.FromAddress)
	if cfg.Host == "" {
		return channel, fmt.Errorf("请填写 SMTP 主机")
	}
	if strings.ContainsAny(cfg.Host, " /\\") {
		return channel, fmt.Errorf("SMTP 主机格式不正确")
	}
	cfg.Security = strings.ToLower(strings.TrimSpace(cfg.Security))
	if cfg.Security == "" {
		cfg.Security = "starttls"
	}
	if !oneOf(cfg.Security, "ssl", "starttls", "none") {
		return channel, fmt.Errorf("加密方式不合法")
	}
	if cfg.Port == 0 {
		cfg.Port = defaultSMTPPort(cfg.Security)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return channel, fmt.Errorf("SMTP 端口需为 1~65535")
	}
	if cfg.TimeoutSec == 0 {
		cfg.TimeoutSec = 10
	}
	if cfg.TimeoutSec < 5 || cfg.TimeoutSec > 60 {
		return channel, fmt.Errorf("超时时间需为 5~60 秒")
	}
	if cfg.Username == "" {
		return channel, fmt.Errorf("请填写 SMTP 用户名")
	}
	if cfg.FromAddress == "" {
		return channel, fmt.Errorf("请填写发件邮箱")
	}
	fromAddress, err := normalizeEmail(cfg.FromAddress)
	if err != nil {
		return channel, fmt.Errorf("发件邮箱格式不正确")
	}
	cfg.FromAddress = fromAddress
	if cfg.FromName == "" {
		cfg.FromName = channel.Name
	}
	if len([]rune(cfg.FromName)) > 64 {
		return channel, fmt.Errorf("发件人名称不能超过 64 个字符")
	}

	if existing != nil {
		oldConfig, decryptErr := s.decryptNotificationConfig(*existing)
		if decryptErr == nil && strings.TrimSpace(cfg.Password) == "" {
			cfg.Password = oldConfig.Password
		}
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return channel, fmt.Errorf("请填写 SMTP 密码或授权码")
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return channel, err
	}
	encrypted, err := s.cipher.Encrypt(string(raw))
	if err != nil {
		return channel, err
	}
	channel.ConfigEnc = encrypted
	return channel, nil
}

func (s *Server) decryptNotificationConfig(channel model.NotificationChannel) (notificationEmailConfig, error) {
	var cfg notificationEmailConfig
	if strings.TrimSpace(channel.ConfigEnc) == "" {
		return cfg, fmt.Errorf("渠道配置为空")
	}
	raw, err := s.cipher.Decrypt(channel.ConfigEnc)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (s *Server) notificationChannelView(channel model.NotificationChannel) notificationChannelView {
	view := notificationChannelView{
		ID: channel.ID, ProjectID: channel.ProjectID, Name: channel.Name,
		Type: channel.Type, Provider: channel.Provider, Status: channel.Status,
		LastTestAt: channel.LastTestAt, LastTestOK: channel.LastTestOK,
		LastTestMessage: channel.LastTestMessage,
		CreatedAt:       channel.CreatedAt, UpdatedAt: channel.UpdatedAt,
	}
	cfg, err := s.decryptNotificationConfig(channel)
	if err != nil {
		return view
	}
	view.HasPassword = strings.TrimSpace(cfg.Password) != ""
	cfg.Password = ""
	view.Config = cfg
	return view
}

func defaultSMTPPort(security string) int {
	switch security {
	case "ssl":
		return 465
	case "none":
		return 25
	default:
		return 587
	}
}

func sendNotificationTestEmail(cfg notificationEmailConfig, channelName, projectName, to string) error {
	return notify.SendEmail(notify.EmailConfig{
		Host: cfg.Host, Port: cfg.Port, Username: cfg.Username, Password: cfg.Password,
		FromName: cfg.FromName, FromAddress: cfg.FromAddress, Security: cfg.Security,
		TimeoutSec: cfg.TimeoutSec,
	}, to, "SmartDoc Vision 通知渠道测试", fmt.Sprintf(
		"这是一封来自 SmartDoc Vision 的测试邮件。\n\n渠道名称：%s\n项目：%s\n发送时间：%s\n",
		channelName, projectName, time.Now().Format("2006-01-02 15:04:05"),
	))
}
