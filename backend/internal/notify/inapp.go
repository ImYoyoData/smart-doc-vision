package notify

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

// NotifyReviewPending creates one inbox message per eligible reviewer when a
// task enters the internal REVIEW_PENDING queue. An empty audit scope means the
// user can access every audit inside their project scope.
func NotifyReviewPending(db *gorm.DB, cipher *security.Cipher, cfg config.Config, task model.Task, audit model.Audit, summary string) error {
	recipients, err := reviewRecipients(db, task)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}
	body := fmt.Sprintf("任务 %s 已进入待议复核，请及时处理。", task.TaskNo)
	if text := strings.TrimSpace(summary); text != "" {
		body += " " + truncateRunes(text, 180)
	}
	title := truncateRunes("待议复核 · "+audit.Name, 200)
	link := fmt.Sprintf("/reviews?project_id=%d&audit_id=%d&task_id=%d", task.ProjectID, task.AuditID, task.ID)
	messages := make([]model.InAppMessage, 0, len(recipients))
	for _, user := range recipients {
		messages = append(messages, model.InAppMessage{
			UserID:    user.ID,
			DedupeKey: fmt.Sprintf("review:%d", task.ID),
			Type:      model.MessageTypeReviewPending,
			Title:     title,
			Body:      truncateRunes(body, 1000),
			Link:      link,
			ProjectID: task.ProjectID,
			AuditID:   task.AuditID,
			TaskID:    task.ID,
		})
	}
	return CreateMessages(db, cipher, task.ProjectID, messages, title, body, cfg.SiteName, cfg.SiteURL)
}

// CreateMessages is the shared entry point for every in-app message type. It
// persists new messages and, for users who enabled email notifications, sends
// the same notification to their email using the project's usable channel.
func CreateMessages(db *gorm.DB, cipher *security.Cipher, projectID uint64, messages []model.InAppMessage, subject, body, siteName, siteURL string) error {
	if len(messages) == 0 {
		return nil
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&messages)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	go sendMessageEmails(db, cipher, projectID, messages, subject, body, siteName, siteURL)
	return nil
}

func sendMessageEmails(db *gorm.DB, cipher *security.Cipher, projectID uint64, messages []model.InAppMessage, subject, body, siteName, siteURL string) {
	if cipher == nil || projectID == 0 {
		return
	}
	userIDs := make([]uint64, 0, len(messages))
	seen := make(map[uint64]bool, len(messages))
	for _, message := range messages {
		if message.UserID == 0 || seen[message.UserID] {
			continue
		}
		seen[message.UserID] = true
		userIDs = append(userIDs, message.UserID)
	}
	var users []model.User
	if err := db.Where("id IN ? AND email_notify_enabled = ? AND email <> ''", userIDs, true).Find(&users).Error; err != nil {
		log.Printf("load email notification recipients: %v", err)
		return
	}
	if len(users) == 0 {
		return
	}
	var channel model.NotificationChannel
	if err := db.Where("project_id = ? AND type = ? AND status = ? AND last_test_ok = ?",
		projectID, model.NotificationTypeEmail, model.NotificationStatusActive, true).
		Order("id asc").First(&channel).Error; err != nil {
		return
	}
	raw, err := cipher.Decrypt(channel.ConfigEnc)
	if err != nil {
		log.Printf("decrypt notification channel %d: %v", channel.ID, err)
		return
	}
	var cfg EmailConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		log.Printf("parse notification channel %d: %v", channel.ID, err)
		return
	}
	userMap := make(map[uint64]model.User, len(users))
	for _, user := range users {
		userMap[user.ID] = user
	}
	for _, message := range messages {
		user, ok := userMap[message.UserID]
		if !ok {
			continue
		}
		mailSubject := subject
		if strings.TrimSpace(mailSubject) == "" {
			mailSubject = message.Title
		}
		mailBody := body
		if strings.TrimSpace(mailBody) == "" {
			mailBody = message.Body
		}
		to := user.Email
		go func() {
			if err := SendEmail(cfg, to, mailSubject, mailBody+"\n\n请登录 SmartDoc Vision 查看详情。"); err != nil {
				log.Printf("send notification email to %s: %v", to, err)
			}
		}()
	}
}

func reviewRecipients(db *gorm.DB, task model.Task) ([]model.User, error) {
	roles, err := loadRoleDefinitions(db)
	if err != nil {
		return nil, err
	}
	var users []model.User
	projectMembers := db.Model(&model.ProjectMember{}).Select("user_id").Where("project_id = ?", task.ProjectID)
	if err := db.Model(&model.User{}).
		Where("status = ?", "ACTIVE").
		Where("role = ? OR id IN (?)", model.RoleSuperAdmin, projectMembers).
		Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	userIDs := make([]uint64, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	var scopes []model.UserAuditScope
	if err := db.Where("user_id IN ?", userIDs).Find(&scopes).Error; err != nil {
		return nil, err
	}
	scopeMap := make(map[uint64][]uint64, len(userIDs))
	for _, scope := range scopes {
		scopeMap[scope.UserID] = append(scopeMap[scope.UserID], scope.AuditID)
	}
	recipients := make([]model.User, 0, len(users))
	for _, user := range users {
		if user.Role != model.RoleSuperAdmin {
			if !roleCanReview(roles[user.Role]) {
				continue
			}
			auditIDs := scopeMap[user.ID]
			if len(auditIDs) > 0 && !containsUint(auditIDs, task.AuditID) {
				continue
			}
		}
		recipients = append(recipients, user)
	}
	return recipients, nil
}

func loadRoleDefinitions(db *gorm.DB) (map[string]catalog.RoleDefinition, error) {
	var records []model.Role
	if err := db.Find(&records).Error; err != nil {
		return nil, err
	}
	definitions := make(map[string]catalog.RoleDefinition, len(records))
	for _, record := range records {
		var permissions, menus []string
		if err := model.DecodeJSON(record.Permissions, &permissions); err != nil {
			continue
		}
		if err := model.DecodeJSON(record.Menus, &menus); err != nil {
			continue
		}
		definitions[record.Code] = catalog.RoleDefinition{
			Code: record.Code, Permissions: permissions, Menus: menus, Builtin: record.Builtin,
		}
	}
	if len(definitions) == 0 {
		for _, definition := range catalog.Roles() {
			definitions[definition.Code] = definition
		}
	}
	return definitions, nil
}

func roleCanReview(definition catalog.RoleDefinition) bool {
	if definition.Code == "" {
		return false
	}
	return containsString(definition.Permissions, "task:review") && containsString(definition.Menus, "reviews")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsUint(values []uint64, target uint64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
