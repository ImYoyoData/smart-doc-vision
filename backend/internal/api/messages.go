package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

type inAppMessageView struct {
	ID        uint64     `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link"`
	ProjectID uint64     `json:"project_id"`
	AuditID   uint64     `json:"audit_id"`
	TaskID    uint64     `json:"task_id"`
	Read      bool       `json:"read"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (s *Server) listMessages(c *gin.Context) {
	user := currentUser(c)
	page, pageSize := pagination(c)
	q := s.db.Model(&model.InAppMessage{}).Where("user_id = ?", user.ID)
	if c.Query("unread") == "true" {
		q = q.Where("read_at IS NULL")
	}
	switch strings.ToUpper(strings.TrimSpace(c.Query("status"))) {
	case "UNREAD":
		q = q.Where("read_at IS NULL")
	case "READ":
		q = q.Where("read_at IS NOT NULL")
	}
	if channelType := strings.TrimSpace(c.Query("type")); channelType != "" {
		q = q.Where("type = ?", channelType)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("(title LIKE ? OR body LIKE ?)", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}
	var unread int64
	if err := s.db.Model(&model.InAppMessage{}).
		Where("user_id = ? AND read_at IS NULL", user.ID).
		Count(&unread).Error; err != nil {
		serverError(c, err)
		return
	}
	var rows []model.InAppMessage
	if err := q.Order("created_at desc, id desc").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		serverError(c, err)
		return
	}
	items := make([]inAppMessageView, 0, len(rows))
	for _, row := range rows {
		items = append(items, inAppMessageView{
			ID: row.ID, Type: row.Type, Title: row.Title, Body: row.Body, Link: row.Link,
			ProjectID: row.ProjectID, AuditID: row.AuditID, TaskID: row.TaskID,
			Read: row.ReadAt != nil, ReadAt: row.ReadAt, CreatedAt: row.CreatedAt,
		})
	}
	ok(c, gin.H{
		"items": items, "total": total, "unread_count": unread,
		"page": page, "page_size": pageSize,
		"has_more": int64(page*pageSize) < total,
	})
}

func (s *Server) markMessageRead(c *gin.Context) {
	user := currentUser(c)
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "消息 ID 无效")
		return
	}
	now := time.Now()
	if err := s.db.Model(&model.InAppMessage{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", id, user.ID).
		Update("read_at", now).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"read": true, "unread_count": s.unreadMessageCount(user.ID)})
}

func (s *Server) markAllMessagesRead(c *gin.Context) {
	user := currentUser(c)
	now := time.Now()
	if err := s.db.Model(&model.InAppMessage{}).
		Where("user_id = ? AND read_at IS NULL", user.ID).
		Update("read_at", now).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"read": true, "unread_count": 0})
}

func (s *Server) unreadMessageCount(userID uint64) int64 {
	var unread int64
	_ = s.db.Model(&model.InAppMessage{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&unread).Error
	return unread
}
