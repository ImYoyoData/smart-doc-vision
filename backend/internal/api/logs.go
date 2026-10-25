package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/service"
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

func (s *Server) purgeRecycleItem(c *gin.Context) {
	user := currentUser(c)
	if !isSuperAdmin(user) {
		forbidden(c, "仅超级管理员可永久删除")
		return
	}
	objectType := strings.ToUpper(strings.TrimSpace(c.Param("type")))
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "对象 ID 无效")
		return
	}
	switch objectType {
	case "PROJECT":
		var project model.Project
		if err := s.db.First(&project, id).Error; err != nil || project.DeletedAt == nil {
			notFound(c, "回收站中的项目不存在")
			return
		}
		if err := service.PurgeProject(s.db, project.ID); err != nil {
			serverError(c, err)
			return
		}
		s.writeAudit(c, &project.ID, "PROJECT_PURGE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS",
			map[string]any{"name": project.Name, "code": project.Code})
		ok(c, gin.H{"purged": true, "type": "PROJECT", "id": project.ID})
	case "AUDIT":
		var audit model.Audit
		if err := s.db.First(&audit, id).Error; err != nil || audit.DeletedAt == nil {
			notFound(c, "回收站中的审核配置不存在")
			return
		}
		if err := service.PurgeAudit(s.db, audit.ID); err != nil {
			serverError(c, err)
			return
		}
		s.writeAudit(c, &audit.ProjectID, "AUDIT_PURGE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS",
			map[string]any{"name": audit.Name, "code": audit.Code})
		ok(c, gin.H{"purged": true, "type": "AUDIT", "id": audit.ID})
	default:
		badRequest(c, "对象类型需为 PROJECT 或 AUDIT")
	}
}

func (s *Server) listRecycleBin(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "recycle:write") {
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

type recycleItemRef struct {
	Type string `json:"type"`
	ID   uint64 `json:"id"`
}

type recycleBatchRequest struct {
	Items     []recycleItemRef `json:"items"`
	CaptchaID string           `json:"captcha_id"`
	CaptchaX  int              `json:"captcha_x"`
}

type recycleBatchResult struct {
	Type   string `json:"type"`
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// verifyRecycleCaptcha enforces the slider second confirmation for destructive
// recycle operations (batch purge / purge-all / batch restore).
func (s *Server) verifyRecycleCaptcha(c *gin.Context, id string, x int) bool {
	if !s.verifyCaptcha(c.Request.Context(), id, x, c.ClientIP()) {
		fail(c, http.StatusBadRequest, "CAPTCHA_INVALID", "滑块验证失败或已过期，请重新拖动", nil)
		return false
	}
	return true
}

func (s *Server) batchRestoreRecycle(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "recycle:write") {
		forbidden(c, "当前角色无权操作回收站")
		return
	}
	var req recycleBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		badRequest(c, "请选择要恢复的对象")
		return
	}
	if len(req.Items) > 200 {
		badRequest(c, "单次最多恢复 200 个对象")
		return
	}
	if !s.verifyRecycleCaptcha(c, req.CaptchaID, req.CaptchaX) {
		return
	}
	results := make([]recycleBatchResult, 0, len(req.Items))
	restored := 0
	// Projects first: restoring a project also restores its cascaded audits,
	// which then skip as "not in recycle bin" below.
	for _, item := range req.Items {
		if strings.ToUpper(item.Type) != "PROJECT" {
			continue
		}
		var project model.Project
		if err := s.db.First(&project, item.ID).Error; err != nil || project.DeletedAt == nil {
			results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Status: "skipped", Reason: "回收站中的项目不存在"})
			continue
		}
		if !isSuperAdmin(user) {
			results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Name: project.Name, Status: "skipped", Reason: "仅超级管理员可恢复项目"})
			continue
		}
		var conflictCount int64
		_ = s.db.Model(&model.Project{}).Where("code = ? AND id <> ? AND deleted_at IS NULL", project.Code, project.ID).Count(&conflictCount).Error
		if conflictCount > 0 {
			results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Name: project.Name, Status: "skipped", Reason: "项目编码已被其他项目占用"})
			continue
		}
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.Project{}).Where("id = ?", project.ID).Updates(map[string]any{
				"status": model.ProjectActive, "deleted_at": nil,
			}).Error; err != nil {
				return err
			}
			return tx.Model(&model.Audit{}).Where("project_id = ? AND deleted_with_project = 1", project.ID).Updates(map[string]any{
				"status": model.AuditActive, "deleted_at": nil, "deleted_with_project": false,
			}).Error
		})
		if err != nil {
			results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Name: project.Name, Status: "skipped", Reason: "恢复失败：" + err.Error()})
			continue
		}
		s.writeAudit(c, &project.ID, "PROJECT_RESTORE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS", nil)
		results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Name: project.Name, Status: "restored"})
		restored++
	}
	for _, item := range req.Items {
		if strings.ToUpper(item.Type) != "AUDIT" {
			continue
		}
		var audit model.Audit
		if err := s.db.First(&audit, item.ID).Error; err != nil || audit.DeletedAt == nil {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Status: "skipped", Reason: "回收站中的审核配置不存在（可能已随项目恢复）"})
			continue
		}
		if !isSuperAdmin(user) && !s.hasProjectAccess(user, audit.ProjectID) {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "skipped", Reason: "无该项目数据权限"})
			continue
		}
		var conflictCount int64
		_ = s.db.Model(&model.Audit{}).Where("project_id = ? AND code = ? AND id <> ? AND deleted_at IS NULL", audit.ProjectID, audit.Code, audit.ID).Count(&conflictCount).Error
		if conflictCount > 0 {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "skipped", Reason: "当前项目下已有同编码审核"})
			continue
		}
		var providerCount int64
		_ = s.db.Model(&model.AIProvider{}).Where("id = ?", audit.ProviderID).Count(&providerCount).Error
		if providerCount == 0 {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "skipped", Reason: "依赖的模型供应商已被删除"})
			continue
		}
		if err := s.db.Model(&model.Audit{}).Where("id = ?", audit.ID).Updates(map[string]any{
			"status": model.AuditActive, "deleted_at": nil, "deleted_with_project": false,
		}).Error; err != nil {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "skipped", Reason: "恢复失败"})
			continue
		}
		s.writeAudit(c, &audit.ProjectID, "AUDIT_RESTORE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS", nil)
		results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "restored"})
		restored++
	}
	ok(c, gin.H{"restored": restored, "total": len(req.Items), "results": results})
}

func (s *Server) batchPurgeRecycle(c *gin.Context) {
	user := currentUser(c)
	if !isSuperAdmin(user) {
		forbidden(c, "仅超级管理员可永久删除")
		return
	}
	var req recycleBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		badRequest(c, "请选择要永久删除的对象")
		return
	}
	if len(req.Items) > 200 {
		badRequest(c, "单次最多永久删除 200 个对象")
		return
	}
	if !s.verifyRecycleCaptcha(c, req.CaptchaID, req.CaptchaX) {
		return
	}
	results := make([]recycleBatchResult, 0, len(req.Items))
	purged := 0
	// Projects first: purging a project cascades its audits away.
	for _, item := range req.Items {
		if strings.ToUpper(item.Type) != "PROJECT" {
			continue
		}
		var project model.Project
		if err := s.db.First(&project, item.ID).Error; err != nil || project.DeletedAt == nil {
			results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Status: "skipped", Reason: "回收站中的项目不存在"})
			continue
		}
		if err := service.PurgeProject(s.db, project.ID); err != nil {
			results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Name: project.Name, Status: "skipped", Reason: "清理失败"})
			continue
		}
		s.writeAudit(c, &project.ID, "PROJECT_PURGE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS",
			map[string]any{"name": project.Name, "code": project.Code, "batch": true})
		results = append(results, recycleBatchResult{Type: "PROJECT", ID: item.ID, Name: project.Name, Status: "purged"})
		purged++
	}
	for _, item := range req.Items {
		if strings.ToUpper(item.Type) != "AUDIT" {
			continue
		}
		var audit model.Audit
		if err := s.db.First(&audit, item.ID).Error; err != nil || audit.DeletedAt == nil {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Status: "skipped", Reason: "回收站中的审核配置不存在（可能已随项目清理）"})
			continue
		}
		if err := service.PurgeAudit(s.db, audit.ID); err != nil {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "skipped", Reason: "清理失败"})
			continue
		}
		s.writeAudit(c, &audit.ProjectID, "AUDIT_PURGE", "AUDIT", fmt.Sprint(audit.ID), "SUCCESS",
			map[string]any{"name": audit.Name, "code": audit.Code, "batch": true})
		results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "purged"})
		purged++
	}
	ok(c, gin.H{"purged": purged, "total": len(req.Items), "results": results})
}

func (s *Server) purgeAllRecycle(c *gin.Context) {
	user := currentUser(c)
	if !isSuperAdmin(user) {
		forbidden(c, "仅超级管理员可清空回收站")
		return
	}
	var req struct {
		CaptchaID string `json:"captcha_id"`
		CaptchaX  int    `json:"captcha_x"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if !s.verifyRecycleCaptcha(c, req.CaptchaID, req.CaptchaX) {
		return
	}
	var projects []model.Project
	if err := s.db.Where("deleted_at IS NOT NULL").Find(&projects).Error; err != nil {
		serverError(c, err)
		return
	}
	var audits []model.Audit
	if err := s.db.Where("deleted_at IS NOT NULL").Find(&audits).Error; err != nil {
		serverError(c, err)
		return
	}
	purgedProjects, purgedAudits := 0, 0
	failed := 0
	projectIDs := make(map[uint64]bool, len(projects))
	for _, p := range projects {
		if err := service.PurgeProject(s.db, p.ID); err != nil {
			failed++
			continue
		}
		projectIDs[p.ID] = true
		purgedProjects++
	}
	for _, a := range audits {
		if projectIDs[a.ProjectID] {
			continue // already cascaded with its project
		}
		if err := service.PurgeAudit(s.db, a.ID); err != nil {
			failed++
			continue
		}
		purgedAudits++
	}
	s.writeAudit(c, nil, "RECYCLE_PURGE_ALL", "RECYCLE", "-", "SUCCESS",
		map[string]any{"projects": purgedProjects, "audits": purgedAudits, "failed": failed})
	ok(c, gin.H{"purged_projects": purgedProjects, "purged_audits": purgedAudits, "failed": failed})
}
