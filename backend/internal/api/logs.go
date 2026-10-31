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

type auditLogFilter struct {
	ProjectID  *uint64
	Actions    []string
	Result     string
	Operator   string
	ObjectType string
	ObjectID   string
	Keyword    string
	TraceID    string
	Start      string
	End        string
}

// auditLogQuery applies the caller's data scope plus the supplied filters.
// Super administrators see all logs (including global login events); other
// users see their projects plus their own project-less login events.
func (s *Server) auditLogQuery(user *model.User, filter auditLogFilter) *gorm.DB {
	q := s.db.Model(&model.AuditLog{})
	if filter.ProjectID != nil {
		q = q.Where("project_id = ?", *filter.ProjectID)
	} else if !isSuperAdmin(user) {
		if len(user.ProjectIDs) == 0 {
			q = q.Where("project_id IS NULL AND operator = ?", user.Username)
		} else {
			q = q.Where("(project_id IN ? OR (project_id IS NULL AND operator = ?))", user.ProjectIDs, user.Username)
		}
	}
	if len(filter.Actions) > 0 {
		q = q.Where("action IN ?", filter.Actions)
	}
	if filter.Result != "" {
		q = q.Where("result = ?", filter.Result)
	}
	if filter.Operator != "" {
		q = q.Where("operator LIKE ?", "%"+filter.Operator+"%")
	}
	if filter.ObjectType != "" {
		q = q.Where("object_type = ?", filter.ObjectType)
	}
	if filter.ObjectID != "" {
		q = q.Where("object_id = ?", filter.ObjectID)
	}
	if filter.TraceID != "" {
		q = q.Where("trace_id = ?", filter.TraceID)
	}
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		q = q.Where("(operator LIKE ? OR action LIKE ? OR object_type LIKE ? OR object_id LIKE ? OR trace_id LIKE ? OR detail_json LIKE ?)",
			like, like, like, like, like, like)
	}
	if t, err := parseAuditDate(filter.Start); err != nil {
		q = q.Where("1 = 0")
	} else if !t.IsZero() {
		q = q.Where("created_at >= ?", t)
	}
	if t, err := parseAuditDate(filter.End); err != nil {
		q = q.Where("1 = 0")
	} else if !t.IsZero() {
		q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
	}
	return q
}

func parseAuditDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", value, time.Local)
}

func (s *Server) listAuditLogs(c *gin.Context) {
	page, pageSize := pagination(c)
	user := currentUser(c)
	filter := auditLogFilter{
		Actions:    splitList(c.Query("action")),
		Result:     strings.TrimSpace(c.Query("result")),
		Operator:   strings.TrimSpace(c.Query("operator")),
		ObjectType: strings.TrimSpace(c.Query("object_type")),
		ObjectID:   strings.TrimSpace(c.Query("object_id")),
		Keyword:    strings.TrimSpace(c.Query("keyword")),
		TraceID:    strings.TrimSpace(c.Query("trace_id")),
		Start:      c.Query("start"),
		End:        c.Query("end"),
	}
	if pid := c.Query("project_id"); pid != "" {
		if id, okID := parseUint(pid); okID {
			if !s.requireProjectAccess(c, id) {
				return
			}
			filter.ProjectID = &id
		}
	}
	q := s.auditLogQuery(user, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}
	var items []model.AuditLog
	if err := q.Order(auditLogOrder(c.Query("sort"))).Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize, "has_more": int64(page*pageSize) < total})
}

type recycleItem struct {
	Type          string        `json:"type"`
	ObjectType    string        `json:"object_type"`
	ID            uint64        `json:"id"`
	Name          string        `json:"name"`
	Code          string        `json:"code"`
	ProjectID     uint64        `json:"project_id"`
	ProjectName   string        `json:"project_name"`
	Status        string        `json:"status"`
	DeletedAt     time.Time     `json:"deleted_at"`
	ExpireAt      time.Time     `json:"expire_at"`
	RemainingDays int           `json:"remaining_days"`
	TaskCount     int64         `json:"task_count"`
	CanRestore    bool          `json:"can_restore"`
	Audits        []recycleItem `json:"audits,omitempty"`
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
	objType := strings.ToUpper(strings.TrimSpace(c.Query("object_type")))
	keyword := strings.TrimSpace(c.Query("keyword"))
	projectFilter := uint64(0)
	if v := c.Query("project_id"); v != "" {
		if id, okID := parseUint(v); okID {
			projectFilter = id
		}
	}
	now := time.Now()

	var deletedProjects []model.Project
	_ = s.db.Where("deleted_at IS NOT NULL").Order("deleted_at desc").Find(&deletedProjects).Error
	var deletedAudits []model.Audit
	_ = s.db.Where("deleted_at IS NOT NULL").Order("deleted_at desc").Find(&deletedAudits).Error
	var allProjects []model.Project
	_ = s.db.Find(&allProjects).Error
	projectsByID := make(map[uint64]model.Project, len(allProjects))
	for _, project := range allProjects {
		projectsByID[project.ID] = project
	}
	auditsByProject := make(map[uint64][]model.Audit)
	for _, audit := range deletedAudits {
		auditsByProject[audit.ProjectID] = append(auditsByProject[audit.ProjectID], audit)
	}

	items := make([]recycleItem, 0)
	includeProjects := objType == "" || objType == "ALL" || objType == "PROJECT"
	includeAudits := objType == "" || objType == "ALL" || objType == "AUDIT"

	if includeProjects {
		for _, project := range deletedProjects {
			if projectFilter != 0 && project.ID != projectFilter {
				continue
			}
			if !isSuperAdmin(user) && !s.hasProjectAccess(user, project.ID) {
				continue
			}
			projectMatched := recycleMatches(project.Name, project.Code, keyword)
			children := make([]recycleItem, 0)
			for _, audit := range auditsByProject[project.ID] {
				// A keyword that matches the project name shows all of its
				// audits; otherwise only audits matching the keyword are kept.
				if keyword != "" && !projectMatched && !recycleMatches(audit.Name, audit.Code, keyword) {
					continue
				}
				expireAt, remaining := recycleWindow(*audit.DeletedAt, project.RetentionDays, now)
				var taskCount int64
				_ = s.db.Model(&model.Task{}).Where("audit_id = ?", audit.ID).Count(&taskCount).Error
				children = append(children, recycleItem{
					Type: "AUDIT", ObjectType: "AUDIT", ID: audit.ID, Name: audit.Name, Code: audit.Code,
					ProjectID: audit.ProjectID, ProjectName: project.Name, Status: audit.Status,
					DeletedAt: *audit.DeletedAt, ExpireAt: expireAt, RemainingDays: remaining,
					TaskCount: taskCount, CanRestore: false,
				})
			}
			if keyword != "" && !projectMatched && len(children) == 0 {
				continue
			}
			expireAt, remaining := recycleWindow(*project.DeletedAt, project.RetentionDays, now)
			var taskCount int64
			_ = s.db.Model(&model.Task{}).Where("project_id = ?", project.ID).Count(&taskCount).Error
			items = append(items, recycleItem{
				Type: "PROJECT", ObjectType: "PROJECT", ID: project.ID, Name: project.Name, Code: project.Code,
				ProjectID: project.ID, ProjectName: project.Name, Status: project.Status,
				DeletedAt: *project.DeletedAt, ExpireAt: expireAt, RemainingDays: remaining,
				TaskCount: taskCount, CanRestore: remaining > 0 && isSuperAdmin(user),
				Audits: children,
			})
		}
	}

	if includeAudits {
		for _, audit := range deletedAudits {
			parent, parentExists := projectsByID[audit.ProjectID]
			// Audits deleted together with a project are only shown under that
			// project, never as a flat row, so they cannot be restored alone.
			if parentExists && parent.DeletedAt != nil {
				continue
			}
			if projectFilter != 0 && audit.ProjectID != projectFilter {
				continue
			}
			if !recycleMatches(audit.Name, audit.Code, keyword) {
				continue
			}
			if !isSuperAdmin(user) && !s.hasProjectAccess(user, audit.ProjectID) {
				continue
			}
			retention := 30
			if parentExists && parent.RetentionDays > 0 {
				retention = parent.RetentionDays
			}
			expireAt, remaining := recycleWindow(*audit.DeletedAt, retention, now)
			var taskCount int64
			_ = s.db.Model(&model.Task{}).Where("audit_id = ?", audit.ID).Count(&taskCount).Error
			items = append(items, recycleItem{
				Type: "AUDIT", ObjectType: "AUDIT", ID: audit.ID, Name: audit.Name, Code: audit.Code,
				ProjectID: audit.ProjectID, ProjectName: parent.Name, Status: audit.Status,
				DeletedAt: *audit.DeletedAt, ExpireAt: expireAt, RemainingDays: remaining,
				TaskCount: taskCount, CanRestore: remaining > 0,
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].DeletedAt.After(items[j].DeletedAt) })
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

func recycleWindow(deletedAt time.Time, retentionDays int, now time.Time) (time.Time, int) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	expireAt := deletedAt.AddDate(0, 0, retentionDays)
	remaining := int(expireAt.Sub(now).Hours() / 24)
	if remaining < 0 {
		remaining = 0
	}
	return expireAt, remaining
}

func recycleMatches(name, code, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), keyword) || strings.Contains(strings.ToLower(code), keyword)
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
		var parentProject model.Project
		if err := s.db.First(&parentProject, audit.ProjectID).Error; err == nil && parentProject.DeletedAt != nil {
			results = append(results, recycleBatchResult{Type: "AUDIT", ID: item.ID, Name: audit.Name, Status: "skipped", Reason: "所属项目仍在回收站中，请先恢复项目"})
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
