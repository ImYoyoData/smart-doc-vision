package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/service"
)

type submitTaskRequest struct {
	ProjectID   uint64            `json:"project_id"`
	AuditID     uint64            `json:"audit_id"`
	BizKey      string            `json:"biz_key"`
	Media       []model.MediaItem `json:"media"`
	Text        string            `json:"text"`
	Images      []string          `json:"images"`
	Ext         map[string]any    `json:"ext"`
	CallbackURL string            `json:"callback_url"`
}

func (s *Server) listTasks(c *gin.Context) {
	user := currentUser(c)
	page, pageSize := pagination(c)
	q := s.db.Model(&model.Task{})
	if pid := c.Query("project_id"); pid != "" {
		id, okID := parseUint(pid)
		if !okID {
			badRequest(c, "project_id 无效")
			return
		}
		if !s.requireProjectAccess(c, id) {
			return
		}
		q = q.Where("project_id = ?", id)
	} else {
		q = s.applyProjectScope(q, user)
	}
	q = s.applyTaskAuditScope(q, user)
	if aid := c.Query("audit_id"); aid != "" {
		id, okID := parseUint(aid)
		if !okID {
			badRequest(c, "audit_id 无效")
			return
		}
		if !s.canAccessAudit(user, id) {
			forbidden(c, "当前账号无权访问该审核配置的任务")
			return
		}
		q = q.Where("audit_id = ?", id)
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status IN ?", splitList(v))
	}
	if v := c.Query("result"); v != "" {
		q = q.Where("result IN ?", splitList(v))
	}
	if v := c.Query("source_type"); v != "" {
		q = q.Where("source_type IN ?", splitList(v))
	}
	if v := c.Query("media_type"); v != "" {
		q = q.Where("media_type IN ?", splitList(v))
	}
	if v := strings.TrimSpace(c.Query("keyword")); v != "" {
		like := "%" + v + "%"
		q = q.Where("(task_no LIKE ? OR biz_key LIKE ?)", like, like)
	}
	if v := strings.TrimSpace(c.Query("biz_key")); v != "" {
		q = q.Where("biz_key = ?", v)
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
	var items []model.Task
	if err := q.Select("id, task_no, project_id, audit_id, source_type, source_id, biz_key, dedupe_hash, media_type, status, result, provider_id, error_message, retry_count, duration_ms, reviewer_id, reviewed_at, started_at, finished_at, created_at, updated_at").
		Order(taskOrder(c.Query("sort"))).Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		serverError(c, err)
		return
	}
	s.populateTaskNames(items)
	ok(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize, "has_more": int64(page*pageSize) < total})
}

func (s *Server) getTask(c *gin.Context) {
	task, err := s.findTask(c)
	if err != nil {
		return
	}
	taskList := []model.Task{*task}
	s.populateTaskNames(taskList)
	*task = taskList[0]
	s.decorateTaskMedia(task)
	var events []model.TaskEvent
	_ = s.db.Where("task_id = ?", task.ID).Order("created_at asc").Find(&events).Error
	ok(c, gin.H{"task": task, "events": events})
}

// decorateTaskMedia fills display URLs for images stored outside the database.
func (s *Server) decorateTaskMedia(task *model.Task) {
	if s.filestore == nil || len(task.InputJSON) == 0 {
		return
	}
	var input model.TaskInput
	if err := model.DecodeJSON(task.InputJSON, &input); err != nil {
		return
	}
	changed := false
	for i := range input.Media {
		key := input.Media[i].Key
		if key == "" {
			continue
		}
		url, err := s.filestore.URL(context.Background(), key, 2*time.Hour)
		if err != nil {
			continue
		}
		input.Media[i].URL = url
		changed = true
	}
	if changed {
		task.InputJSON = model.JSONBytes(input)
	}
}

func (s *Server) submitTask(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "task:submit") {
		forbidden(c, "当前角色无权提交审核任务")
		return
	}
	var req submitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if !s.requireProjectAccess(c, req.ProjectID) {
		return
	}
	if !s.requireAuditScope(c, req.AuditID) {
		return
	}
	input := model.TaskInput{
		BizKey:      strings.TrimSpace(req.BizKey),
		Media:       normalizeMedia(req),
		Ext:         req.Ext,
		CallbackURL: strings.TrimSpace(req.CallbackURL),
	}
	outcome, err := s.tasks.Submit(c.Request.Context(), service.SubmitInput{
		ProjectID: req.ProjectID, AuditID: req.AuditID,
		SourceType: "ADMIN", Input: input,
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
	list := []model.Task{task}
	s.populateTaskNames(list)
	task = list[0]
	if outcome.Duplicated {
		s.writeAudit(c, &task.ProjectID, "TASK_DUPLICATE", "TASK", task.TaskNo, "SUCCESS", map[string]any{
			"audit_id": task.AuditID, "media_type": task.MediaType,
		})
		ok(c, gin.H{"task": task, "duplicated": true})
		return
	}
	s.writeAudit(c, &task.ProjectID, "TASK_SUBMIT", "TASK", task.TaskNo, "SUCCESS", map[string]any{
		"audit_id": task.AuditID, "media_type": task.MediaType, "source_type": task.SourceType,
	})
	created(c, gin.H{"task": task, "duplicated": false})
}

func (s *Server) retryTask(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "task:retry") {
		forbidden(c, "当前角色无权重试任务")
		return
	}
	task, err := s.findTask(c)
	if err != nil {
		return
	}
	if err := s.resetForRetry(task); err != nil {
		badRequest(c, err.Error())
		return
	}
	s.writeAudit(c, &task.ProjectID, "TASK_RETRY", "TASK", task.TaskNo, "SUCCESS", map[string]any{"retry_count": task.RetryCount + 1})
	ok(c, gin.H{"task_id": task.ID, "status": model.TaskQueued})
}

func (s *Server) batchRetryTasks(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "task:retry") {
		forbidden(c, "当前角色无权批量重试")
		return
	}
	var req struct {
		TaskIDs []uint64 `json:"task_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.TaskIDs) == 0 {
		badRequest(c, "请选择需要重试的任务")
		return
	}
	if len(req.TaskIDs) > 50 {
		badRequest(c, "单次最多批量重试 50 条任务")
		return
	}
	success, failed := 0, []gin.H{}
	for _, id := range req.TaskIDs {
		var task model.Task
		if err := s.db.First(&task, id).Error; err != nil {
			failed = append(failed, gin.H{"id": id, "reason": "任务不存在"})
			continue
		}
		if !s.hasProjectAccess(user, task.ProjectID) {
			failed = append(failed, gin.H{"id": id, "reason": "无项目访问权限"})
			continue
		}
		if !s.canAccessAudit(user, task.AuditID) {
			failed = append(failed, gin.H{"id": id, "reason": "无审核配置访问权限"})
			continue
		}
		if err := s.resetForRetry(&task); err != nil {
			failed = append(failed, gin.H{"id": id, "reason": err.Error()})
			continue
		}
		success++
	}
	s.writeAudit(c, nil, "TASK_BATCH_RETRY", "TASK", "", "SUCCESS", map[string]any{"success": success, "failed": len(failed)})
	ok(c, gin.H{"success": success, "failed": failed})
}

func (s *Server) resetForRetry(task *model.Task) error {
	if task.Status != model.TaskFailed && task.Status != model.TaskStopped {
		return fmt.Errorf("仅失败或已停止的任务可以重试")
	}
	res := s.db.Model(&model.Task{}).Where("id = ? AND status = ?", task.ID, task.Status).Updates(map[string]any{
		"status": model.TaskQueued, "result": model.ResultUnknown, "error_message": "",
		"model_result": nil, "rule_trace": nil, "result_json": nil,
		"retry_count": task.RetryCount + 1, "started_at": nil, "finished_at": nil,
		"reviewer_id": nil, "reviewed_at": nil, "review_note": "",
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("任务状态已变更，请刷新后重试")
	}
	ev := model.TaskEvent{
		TaskID: task.ID, ProjectID: task.ProjectID, FromStatus: task.Status,
		ToStatus: model.TaskQueued, Message: "手动重试，重新进入队列",
	}
	_ = s.db.Create(&ev).Error
	if err := s.engine.Enqueue(context.Background(), task.ProjectID, task.ID); err != nil {
		return fmt.Errorf("重新入队失败: %w", err)
	}
	return nil
}

func (s *Server) reviewTask(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "task:review") {
		forbidden(c, "当前角色无权进行人工复核")
		return
	}
	task, err := s.findTask(c)
	if err != nil {
		return
	}
	if task.Status != model.TaskReviewPending {
		badRequest(c, "仅待议状态的任务可进行人工复核")
		return
	}
	var req struct {
		Conclusion string `json:"conclusion" binding:"required"`
		ReasonCode string `json:"reason_code"`
		Remark     string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请选择复核结论")
		return
	}
	req.Conclusion = strings.ToUpper(strings.TrimSpace(req.Conclusion))
	if req.Conclusion != model.ResultPass && req.Conclusion != model.ResultReject {
		badRequest(c, "复核结论必须为 PASS 或 REJECT")
		return
	}
	now := time.Now()
	var detail map[string]any
	_ = model.DecodeJSON(task.ResultJSON, &detail)
	if detail == nil {
		detail = map[string]any{}
	}
	detail["manual_review"] = map[string]any{
		"reviewer_id": user.ID, "reviewer": user.Username,
		"conclusion": req.Conclusion, "reason_code": req.ReasonCode,
		"remark": req.Remark, "reviewed_at": now,
	}
	updates := map[string]any{
		"status": model.TaskSuccess, "result": req.Conclusion,
		"result_json": model.JSONBytes(detail), "reviewer_id": user.ID,
		"reviewed_at": now, "review_note": truncateText(req.Remark, 500),
		"review_reason_code": req.ReasonCode,
		"finished_at":        now, "updated_at": now,
	}
	ev := model.TaskEvent{
		TaskID: task.ID, ProjectID: task.ProjectID, FromStatus: model.TaskReviewPending,
		ToStatus: model.TaskSuccess,
		Message:  fmt.Sprintf("人工复核：%s，原因码=%s，备注=%s", req.Conclusion, emptyDefault(req.ReasonCode, "-"), truncateText(req.Remark, 100)),
	}
	var rowsAffected int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Task{}).Where("id = ? AND status = ?", task.ID, model.TaskReviewPending).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		rowsAffected = res.RowsAffected
		if rowsAffected == 0 {
			return nil
		}
		return tx.Create(&ev).Error
	})
	if err != nil {
		serverError(c, err)
		return
	}
	if rowsAffected == 0 {
		conflict(c, "STATUS_CONFLICT", "任务状态已变更，请刷新")
		return
	}
	s.writeAudit(c, &task.ProjectID, "TASK_REVIEW", "TASK", task.TaskNo, "SUCCESS", map[string]any{
		"conclusion": req.Conclusion, "reason_code": req.ReasonCode, "remark": req.Remark,
	})
	s.engine.ExecuteRoutes(task.ID)
	s.engine.RunCallback(task.ID)
	ok(c, gin.H{"task_id": task.ID, "status": model.TaskSuccess, "result": req.Conclusion})
}

func (s *Server) findTask(c *gin.Context) (*model.Task, error) {
	id, okID := parseUint(c.Param("id"))
	if !okID {
		badRequest(c, "任务 ID 无效")
		return nil, fmt.Errorf("invalid id")
	}
	var task model.Task
	if err := s.db.First(&task, id).Error; err != nil {
		notFound(c, "任务不存在")
		return nil, err
	}
	if !s.requireProjectAccess(c, task.ProjectID) {
		return nil, fmt.Errorf("task out of scope")
	}
	if !s.requireAuditScope(c, task.AuditID) {
		return nil, fmt.Errorf("task audit out of scope")
	}
	return &task, nil
}

func (s *Server) populateTaskNames(tasks []model.Task) {
	auditCache := map[uint64]model.Audit{}
	projectCache := map[uint64]string{}
	providerCache := map[uint64]string{}
	for i := range tasks {
		t := &tasks[i]
		if audit, exists := auditCache[t.AuditID]; exists {
			t.AuditName = audit.Name
		} else {
			var audit model.Audit
			_ = s.db.First(&audit, t.AuditID).Error
			auditCache[t.AuditID] = audit
			t.AuditName = audit.Name
		}
		if name, exists := projectCache[t.ProjectID]; exists {
			t.ProjectName = name
		} else {
			var project model.Project
			_ = s.db.First(&project, t.ProjectID).Error
			projectCache[t.ProjectID] = project.Name
			t.ProjectName = project.Name
		}
		if name, exists := providerCache[t.ProviderID]; exists {
			t.ProviderName = name
		} else {
			var provider model.AIProvider
			_ = s.db.First(&provider, t.ProviderID).Error
			providerCache[t.ProviderID] = provider.Name
			t.ProviderName = provider.Name
		}
	}
}

func normalizeMedia(req submitTaskRequest) []model.MediaItem {
	out := make([]model.MediaItem, 0, len(req.Media)+1)
	for _, item := range req.Media {
		item.Type = strings.ToLower(strings.TrimSpace(item.Type))
		item.Content = strings.TrimSpace(item.Content)
		item.URL = strings.TrimSpace(item.URL)
		if item.Type == "text" && item.Content == "" {
			continue
		}
		// 图片接受 url 或 content 内联 data URL，两者都缺才丢弃。
		if item.Type == "image" && item.URL == "" && !strings.HasPrefix(item.Content, "data:image/") {
			continue
		}
		out = append(out, item)
	}
	if text := strings.TrimSpace(req.Text); text != "" {
		out = append(out, model.MediaItem{Type: "text", Content: text, Name: "document.txt"})
	}
	for _, img := range req.Images {
		if img = strings.TrimSpace(img); img != "" {
			out = append(out, model.MediaItem{Type: "image", URL: img})
		}
	}
	return out
}

func taskOrder(sort string) string {
	allowed := map[string]string{
		"created_at": "created_at", "finished_at": "finished_at", "duration_ms": "duration_ms",
		"status": "status", "result": "result", "task_no": "task_no",
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

func prefixScheme(raw string) string {
	if idx := strings.Index(raw, "://"); idx > 0 {
		return raw[:idx]
	}
	return ""
}

func emptyDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
