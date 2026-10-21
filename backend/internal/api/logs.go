package api

import (
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/model"
)

func (s *Server) listAuditLogs(c *gin.Context) {
	page, pageSize := pagination(c)
	user := currentUser(c)
	q := s.db.Model(&model.AuditLog{})
	if pid := c.Query("project_id"); pid != "" {
		if id, okID := parseUint(pid); okID {
			if !s.requireProjectAccess(c, id) {
				return
			}
			q = q.Where("project_id = ?", id)
		}
	} else if !isSuperAdmin(user) {
		if len(user.ProjectIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("project_id IN ?", user.ProjectIDs)
		}
	}
	if v := c.Query("action"); v != "" {
		q = q.Where("action IN ?", splitList(v))
	}
	if v := c.Query("result"); v != "" {
		q = q.Where("result = ?", v)
	}
	if v := strings.TrimSpace(c.Query("operator")); v != "" {
		q = q.Where("operator LIKE ?", "%"+v+"%")
	}
	if v := strings.TrimSpace(c.Query("object_type")); v != "" {
		q = q.Where("object_type = ?", v)
	}
	if v := strings.TrimSpace(c.Query("object_id")); v != "" {
		q = q.Where("object_id = ?", v)
	}
	if v := c.Query("start"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if v := c.Query("end"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}
	var items []model.AuditLog
	if err := q.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize, "has_more": int64(page*pageSize) < total})
}

type recycleItem struct {
	Type          string    `json:"type"`
	ObjectType    string    `json:"object_type"`
	ID            uint64    `json:"id"`
	Name          string    `json:"name"`
	Code          string    `json:"code"`
	ProjectID     uint64    `json:"project_id"`
	ProjectName   string    `json:"project_name"`
	Status        string    `json:"status"`
	DeletedAt     time.Time `json:"deleted_at"`
	ExpireAt      time.Time `json:"expire_at"`
	RemainingDays int       `json:"remaining_days"`
	TaskCount     int64     `json:"task_count"`
	CanRestore    bool      `json:"can_restore"`
}

func (s *Server) listRecycleBin(c *gin.Context) {
	user := currentUser(c)
	if !canAdmin(user.Role) {
		forbidden(c, "当前角色无权访问回收站")
		return
	}
	items := make([]recycleItem, 0)
	objType := strings.ToUpper(strings.TrimSpace(c.Query("object_type")))
	keyword := strings.TrimSpace(c.Query("keyword"))
	projectFilter := uint64(0)
	if v := c.Query("project_id"); v != "" {
		if id, okID := parseUint(v); okID {
			projectFilter = id
		}
	}
	now := time.Now()

	if objType == "" || objType == "ALL" || objType == "PROJECT" {
		q := s.db.Where("deleted_at IS NOT NULL")
		if keyword != "" {
			like := "%" + keyword + "%"
			q = q.Where("(name LIKE ? OR code LIKE ?)", like, like)
		}
		if projectFilter != 0 {
			q = q.Where("id = ?", projectFilter)
		}
		var projects []model.Project
		_ = q.Order("deleted_at desc").Find(&projects).Error
		for _, p := range projects {
			expire := p.DeletedAt.AddDate(0, 0, p.RetentionDays)
			days := int(time.Until(expire).Hours() / 24)
			if days < 0 {
				days = 0
			}
			var taskCount int64
			_ = s.db.Model(&model.Task{}).Where("project_id = ?", p.ID).Count(&taskCount).Error
			if !isSuperAdmin(user) && !s.hasProjectAccess(user, p.ID) {
				continue
			}
			items = append(items, recycleItem{
				Type: "PROJECT", ObjectType: "PROJECT", ID: p.ID, Name: p.Name, Code: p.Code,
				ProjectID: p.ID, ProjectName: p.Name, Status: p.Status,
				DeletedAt: *p.DeletedAt, ExpireAt: expire, RemainingDays: days,
				TaskCount: taskCount, CanRestore: days > 0,
			})
		}
	}
	if objType == "" || objType == "ALL" || objType == "AUDIT" {
		q := s.db.Model(&model.Audit{}).Where("deleted_at IS NOT NULL")
		if keyword != "" {
			like := "%" + keyword + "%"
			q = q.Where("(name LIKE ? OR code LIKE ?)", like, like)
		}
		if projectFilter != 0 {
			q = q.Where("project_id = ?", projectFilter)
		}
		var audits []model.Audit
		_ = q.Order("deleted_at desc").Find(&audits).Error
		for _, a := range audits {
			var project model.Project
			_ = s.db.First(&project, a.ProjectID).Error
			retention := project.RetentionDays
			if retention <= 0 {
				retention = 30
			}
			expire := a.DeletedAt.AddDate(0, 0, retention)
			days := int(time.Until(expire).Hours() / 24)
			if days < 0 {
				days = 0
			}
			if !isSuperAdmin(user) && !s.hasProjectAccess(user, a.ProjectID) {
				continue
			}
			var taskCount int64
			_ = s.db.Model(&model.Task{}).Where("audit_id = ?", a.ID).Count(&taskCount).Error
			items = append(items, recycleItem{
				Type: "AUDIT", ObjectType: "AUDIT", ID: a.ID, Name: a.Name, Code: a.Code,
				ProjectID: a.ProjectID, ProjectName: project.Name, Status: a.Status,
				DeletedAt: *a.DeletedAt, ExpireAt: expire, RemainingDays: days,
				TaskCount: taskCount, CanRestore: days > 0,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DeletedAt.After(items[j].DeletedAt) })
	page, pageSize := pagination(c)
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	ok(c, gin.H{
		"items": items[start:end], "total": total, "page": page, "page_size": pageSize,
		"has_more":     end < total,
		"current_time": now,
	})
}
