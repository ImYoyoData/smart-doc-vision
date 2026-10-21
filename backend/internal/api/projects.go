package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
)

var projectCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)

type projectInput struct {
	Name             string `json:"name"`
	Code             string `json:"code"`
	Env              string `json:"env"`
	Description      string `json:"description"`
	OwnerID          uint64 `json:"owner_id"`
	RetentionDays    int    `json:"retention_days"`
	MaxTasksPerDay   *int   `json:"max_tasks_per_day"`
	OpenAPIRateLimit *int   `json:"openapi_rate_limit"`
}

func isSuperAdmin(user *model.User) bool {
	return user != nil && user.Role == model.RoleSuperAdmin
}

func (s *Server) hasProjectAccess(user *model.User, projectID uint64) bool {
	if user == nil {
		return false
	}
	if isSuperAdmin(user) {
		return true
	}
	for _, id := range user.ProjectIDs {
		if id == projectID {
			return true
		}
	}
	return false
}

func (s *Server) requireProjectAccess(c *gin.Context, projectID uint64) bool {
	user := currentUser(c)
	if !s.hasProjectAccess(user, projectID) {
		forbidden(c, "没有该项目的访问权限")
		return false
	}
	return true
}

func (s *Server) applyProjectScope(q *gorm.DB, user *model.User) *gorm.DB {
	if isSuperAdmin(user) {
		return q
	}
	if len(user.ProjectIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("project_id IN ?", user.ProjectIDs)
}

func (s *Server) listProjects(c *gin.Context) {
	page, pageSize := pagination(c)
	user := currentUser(c)
	q := s.db.Model(&model.Project{}).Where("deleted_at IS NULL")
	if !isSuperAdmin(user) {
		if len(user.ProjectIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("id IN ?", user.ProjectIDs)
		}
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q = q.Where("status IN ?", splitList(status))
	}
	if env := strings.TrimSpace(c.Query("env")); env != "" {
		q = q.Where("env IN ?", splitList(env))
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR code LIKE ?", like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		serverError(c, err)
		return
	}
	var items []model.Project
	if err := q.Order(projectOrder(c.Query("sort"))).Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		serverError(c, err)
		return
	}
	s.populateProjectStats(items)
	ok(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize, "has_more": int64(page*pageSize) < total})
}

func (s *Server) getProject(c *gin.Context) {
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	list := []model.Project{*project}
	s.populateProjectStats(list)
	*project = list[0]
	ok(c, project)
}

func (s *Server) createProject(c *gin.Context) {
	user := currentUser(c)
	if !isSuperAdmin(user) {
		forbidden(c, "仅超级管理员可创建项目")
		return
	}
	var req projectInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if len([]rune(req.Name)) < 2 || len([]rune(req.Name)) > 64 {
		badRequest(c, "项目名称需为 2~64 个字符")
		return
	}
	if !projectCodePattern.MatchString(req.Code) {
		badRequest(c, "项目编码需以小写字母开头，仅含小写字母、数字、下划线或连字符")
		return
	}
	if req.Env == "" {
		req.Env = "prod"
	}
	if !oneOf(req.Env, "dev", "test", "staging", "prod") {
		badRequest(c, "环境取值不合法")
		return
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = 30
	}
	if req.RetentionDays < 7 || req.RetentionDays > 365 {
		badRequest(c, "回收站保留天数需在 7~365 之间")
		return
	}
	maxTasksPerDay := 0
	if req.MaxTasksPerDay != nil {
		maxTasksPerDay = *req.MaxTasksPerDay
	}
	rateLimit := 120
	if req.OpenAPIRateLimit != nil {
		rateLimit = *req.OpenAPIRateLimit
	}
	if maxTasksPerDay < 0 || maxTasksPerDay > 100000 {
		badRequest(c, "每日任务配额需在 0~100000 之间（0 表示不限制）")
		return
	}
	if rateLimit < 0 || rateLimit > 10000 {
		badRequest(c, "开放 API 每分钟限流需在 0~10000 之间（0 表示不限制）")
		return
	}
	var count int64
	_ = s.db.Model(&model.Project{}).Where("code = ?", req.Code).Count(&count).Error
	if count > 0 {
		conflict(c, "CODE_CONFLICT", "项目编码已被占用（回收站中的对象也计入）")
		return
	}
	ownerID := req.OwnerID
	if ownerID == 0 {
		ownerID = user.ID
	}
	project := model.Project{
		Name: req.Name, Code: req.Code, Env: req.Env, Status: model.ProjectActive,
		Description: strings.TrimSpace(req.Description), OwnerID: ownerID,
		RetentionDays: req.RetentionDays, MaxTasksPerDay: maxTasksPerDay,
		OpenAPIRateLimit: rateLimit, CreatedBy: user.ID,
	}
	if err := s.db.Create(&project).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "PROJECT_CREATE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS",
		map[string]any{"name": project.Name, "code": project.Code, "env": project.Env})
	created(c, project)
}

func (s *Server) updateProject(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权修改项目")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	var req projectInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		if len([]rune(name)) < 2 || len([]rune(name)) > 64 {
			badRequest(c, "项目名称需为 2~64 个字符")
			return
		}
		project.Name = name
	}
	if req.Env != "" {
		if !oneOf(req.Env, "dev", "test", "staging", "prod") {
			badRequest(c, "环境取值不合法")
			return
		}
		project.Env = req.Env
	}
	if req.Description != "" {
		project.Description = req.Description
	}
	if req.RetentionDays != 0 {
		if req.RetentionDays < 7 || req.RetentionDays > 365 {
			badRequest(c, "回收站保留天数需在 7~365 之间")
			return
		}
		project.RetentionDays = req.RetentionDays
	}
	if req.OwnerID != 0 {
		project.OwnerID = req.OwnerID
	}
	if req.MaxTasksPerDay != nil {
		if *req.MaxTasksPerDay < 0 || *req.MaxTasksPerDay > 100000 {
			badRequest(c, "每日任务配额需在 0~100000 之间（0 表示不限制）")
			return
		}
		project.MaxTasksPerDay = *req.MaxTasksPerDay
	}
	if req.OpenAPIRateLimit != nil {
		if *req.OpenAPIRateLimit < 0 || *req.OpenAPIRateLimit > 10000 {
			badRequest(c, "开放 API 每分钟限流需在 0~10000 之间（0 表示不限制）")
			return
		}
		project.OpenAPIRateLimit = *req.OpenAPIRateLimit
	}
	if err := s.db.Save(project).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "PROJECT_UPDATE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS",
		map[string]any{"name": project.Name, "env": project.Env})
	ok(c, project)
}

func (s *Server) updateProjectStatus(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "project:update") {
		forbidden(c, "当前角色无权变更项目状态")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !oneOf(req.Status, model.ProjectActive, model.ProjectPaused, model.ProjectDisabled) {
		badRequest(c, "状态需为 ACTIVE / PAUSED / DISABLED")
		return
	}
	if err := s.db.Model(project).Update("status", req.Status).Error; err != nil {
		serverError(c, err)
		return
	}
	project.Status = req.Status
	s.writeAudit(c, &project.ID, "PROJECT_STATUS", "PROJECT", fmt.Sprint(project.ID), "SUCCESS",
		map[string]any{"status": req.Status, "reason": req.Reason})
	ok(c, project)
}

func (s *Server) deleteProject(c *gin.Context) {
	user := currentUser(c)
	if !isSuperAdmin(user) {
		forbidden(c, "仅超级管理员可删除项目")
		return
	}
	project, err := s.findProject(c)
	if err != nil {
		return
	}
	now := time.Now()
	var audits int64
	_ = s.db.Model(&model.Audit{}).Where("project_id = ? AND deleted_at IS NULL", project.ID).Count(&audits).Error
	var running int64
	_ = s.db.Model(&model.Task{}).Where("project_id = ? AND status IN ?", project.ID,
		[]string{model.TaskQueued, model.TaskProcessing, model.TaskReviewPending}).Count(&running).Error
	tx := s.db.Begin()
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	if err := tx.Model(&model.Project{}).Where("id = ?", project.ID).Updates(map[string]any{
		"status": model.ProjectDeleted, "deleted_at": now,
	}).Error; err != nil {
		tx.Rollback()
		serverError(c, err)
		return
	}
	if err := tx.Model(&model.Audit{}).Where("project_id = ? AND deleted_at IS NULL", project.ID).Updates(map[string]any{
		"status": model.AuditDeleted, "deleted_at": now, "deleted_with_project": true,
	}).Error; err != nil {
		tx.Rollback()
		serverError(c, err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "PROJECT_DELETE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS",
		map[string]any{"affected_audits": audits, "running_tasks": running, "snapshot": project.Name})
	expireAt := now.AddDate(0, 0, project.RetentionDays)
	ok(c, gin.H{"recycle": true, "affected": gin.H{"audits": audits}, "running_tasks": running, "expire_at": expireAt})
}

func (s *Server) restoreProject(c *gin.Context) {
	user := currentUser(c)
	if !isSuperAdmin(user) {
		forbidden(c, "仅超级管理员可恢复项目")
		return
	}
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "项目 ID 无效")
		return
	}
	var project model.Project
	if err := s.db.First(&project, id).Error; err != nil {
		notFound(c, "项目不存在")
		return
	}
	if project.DeletedAt == nil {
		badRequest(c, "项目未在回收站中")
		return
	}
	var conflictCount int64
	_ = s.db.Model(&model.Project{}).Where("code = ? AND id <> ? AND deleted_at IS NULL", project.Code, project.ID).Count(&conflictCount).Error
	if conflictCount > 0 {
		conflict(c, "CODE_CONFLICT", "项目编码已被其他项目占用")
		return
	}
	tx := s.db.Begin()
	if tx.Error != nil {
		serverError(c, tx.Error)
		return
	}
	if err := tx.Model(&model.Project{}).Where("id = ?", project.ID).Updates(map[string]any{
		"status": model.ProjectActive, "deleted_at": nil,
	}).Error; err != nil {
		tx.Rollback()
		serverError(c, err)
		return
	}
	if err := tx.Model(&model.Audit{}).Where("project_id = ? AND deleted_with_project = 1", project.ID).Updates(map[string]any{
		"status": model.AuditActive, "deleted_at": nil, "deleted_with_project": false,
	}).Error; err != nil {
		tx.Rollback()
		serverError(c, err)
		return
	}
	if err := tx.Commit().Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, &project.ID, "PROJECT_RESTORE", "PROJECT", fmt.Sprint(project.ID), "SUCCESS", nil)
	ok(c, gin.H{"restored": true})
}

func (s *Server) findProject(c *gin.Context) (*model.Project, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "项目 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var project model.Project
	if err := s.db.First(&project, id).Error; err != nil {
		notFound(c, "项目不存在")
		return nil, err
	}
	if !s.requireProjectAccess(c, project.ID) {
		return nil, fmt.Errorf("project out of scope")
	}
	return &project, nil
}

func (s *Server) populateProjectStats(items []model.Project) {
	for i := range items {
		p := &items[i]
		_ = s.db.Model(&model.Audit{}).Where("project_id = ? AND deleted_at IS NULL", p.ID).Count(&p.AuditCount).Error
		_ = s.db.Model(&model.AIProvider{}).Where("project_id = ?", p.ID).Count(&p.ProviderCount).Error
		_ = s.db.Model(&model.Task{}).Where("project_id = ? AND created_at >= ?", p.ID, startOfToday()).Count(&p.TodayTasks).Error
	}
}

func projectOrder(sort string) string {
	allowed := map[string]string{
		"created_at":    "created_at",
		"updated_at":    "updated_at",
		"name":          "name",
		"code":          "code",
		"status":        "status",
		"today_tasks":   "created_at",
		"audit_count":   "created_at",
		"queue_backlog": "created_at",
	}
	parts := strings.Split(sort, ":")
	field := allowed[strings.TrimSpace(parts[0])]
	if field == "" {
		return "created_at DESC"
	}
	dir := "DESC"
	if len(parts) > 1 && strings.EqualFold(parts[1], "asc") {
		dir = "ASC"
	}
	return field + " " + dir
}

func pagination(c *gin.Context) (int, int) {
	page := 1
	pageSize := 20
	if v := c.Query("page"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			pageSize = n
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func startOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func oneOf(v string, options ...string) bool {
	for _, opt := range options {
		if v == opt {
			return true
		}
	}
	return false
}

func splitList(v string) []string {
	raw := strings.Split(v, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n, err
}

func (s *Server) noContent(c *gin.Context) { c.Status(http.StatusNoContent) }
