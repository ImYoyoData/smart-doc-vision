package api

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/engine"
	"smartdoc-vision/backend/internal/model"
)

type scriptSimulationTask struct {
	TaskNo     string `json:"task_no"`
	RequestID  string `json:"request_id"`
	BizKey     string `json:"biz_key"`
	SourceType string `json:"source_type"`
	SourceID   uint64 `json:"source_id"`
	SourceRef  string `json:"source_ref"`
	Status     string `json:"status"`
	ReviewNote string `json:"review_note"`
}

type scriptSimulationInput struct {
	Text string         `json:"text"`
	Ext  map[string]any `json:"ext"`
}

type scriptSimulationRequest struct {
	ProjectID uint64                `json:"project_id"`
	AuditID   uint64                `json:"audit_id"`
	Script    string                `json:"script"`
	State     string                `json:"state"`
	Task      scriptSimulationTask  `json:"task"`
	Detail    map[string]any        `json:"detail"`
	Input     scriptSimulationInput `json:"input"`
	TimeoutMS int                   `json:"timeout_ms"`
	MemoryMB  int                   `json:"memory_mb"`
}

func (s *Server) simulateScript(c *gin.Context) {
	user := currentUser(c)
	if !s.hasPermission(user.Role, "audit:write") {
		forbidden(c, "当前角色无权模拟运行脚本")
		return
	}
	var req scriptSimulationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "参数格式错误")
		return
	}
	if req.ProjectID == 0 {
		badRequest(c, "请选择项目")
		return
	}
	if !s.requireProjectAccess(c, req.ProjectID) {
		return
	}
	if strings.TrimSpace(req.Script) == "" {
		badRequest(c, "脚本为空，请先编写代码")
		return
	}
	if len(req.Script) > maxScriptBytes {
		badRequest(c, "脚本超过 64KB 限制")
		return
	}
	state := strings.ToUpper(strings.TrimSpace(req.State))
	if state == "" {
		state = model.ResultReview
	}
	if !oneOf(state, model.ResultPass, model.ResultReject, model.ResultReview) {
		badRequest(c, "模拟状态必须是 PASS / REJECT / REVIEW")
		return
	}
	var project model.Project
	if err := s.db.First(&project, req.ProjectID).Error; err != nil {
		notFound(c, "项目不存在")
		return
	}
	audit := model.Audit{ProjectID: req.ProjectID, Code: "simulation", Name: "模拟运行"}
	if req.AuditID > 0 {
		if err := s.db.Where("id = ? AND project_id = ? AND deleted_at IS NULL", req.AuditID, req.ProjectID).First(&audit).Error; err != nil {
			notFound(c, "审核配置不存在")
			return
		}
	}
	taskNo := strings.TrimSpace(req.Task.TaskNo)
	if taskNo == "" {
		taskNo = "SIM-" + time.Now().Format("20060102-150405")
	}
	bizKey := strings.TrimSpace(req.Task.BizKey)
	if bizKey == "" {
		bizKey = "SIM-BIZ-001"
	}
	sourceType := strings.TrimSpace(req.Task.SourceType)
	if sourceType == "" {
		sourceType = model.TriggerOpenAPI
	}
	status := strings.TrimSpace(req.Task.Status)
	if status == "" {
		status = model.TaskSuccess
	}
	requestID := strings.TrimSpace(req.Task.RequestID)
	var requestIDPtr *string
	if requestID != "" {
		requestIDPtr = &requestID
	}
	task := model.Task{
		ID: 1, TaskNo: taskNo, ProjectID: req.ProjectID, AuditID: audit.ID,
		SourceType: sourceType, SourceID: req.Task.SourceID, BizKey: bizKey, RequestID: requestIDPtr,
		MediaType: "text", Status: status, Result: state,
		ReviewNote: strings.TrimSpace(req.Task.ReviewNote),
		CreatedAt:  time.Now(),
	}
	input := model.TaskInput{BizKey: bizKey, RequestID: requestID, SourceRef: strings.TrimSpace(req.Task.SourceRef), Ext: req.Input.Ext}
	if input.Ext == nil {
		input.Ext = map[string]any{"simulated": true}
	}
	if text := strings.TrimSpace(req.Input.Text); text != "" {
		input.Media = []model.MediaItem{{Type: "text", Content: text}}
	}
	detail := simulationDetail(state)
	for key, value := range req.Detail {
		detail[key] = value
	}
	if raw, err := json.Marshal(detail); err != nil || len(raw) > 64<<10 {
		badRequest(c, "模拟上下文超过 64KB 限制")
		return
	}
	result := s.engine.SimulateScript(c.Request.Context(), engine.ScriptSimulationOptions{
		Project: project, Audit: audit, Task: task, Input: input,
		Result: state, Detail: detail, Script: req.Script,
		MemoryMB: req.MemoryMB, TimeoutMS: req.TimeoutMS,
	})
	ok(c, result)
}

func simulationDetail(state string) map[string]any {
	detail := map[string]any{
		"summary": "模拟运行：待议复核", "risk_level": "medium", "score": 0.55,
		"reasons": []string{"模拟运行存在不确定项"}, "labels": []string{"模拟运行"},
		"category": "", "category_name": "", "category_confidence": 0,
	}
	switch state {
	case model.ResultPass:
		detail["summary"] = "模拟运行：审核通过"
		detail["risk_level"] = "low"
		detail["score"] = 0.06
		detail["reasons"] = []string{"模拟运行未命中风险规则"}
	case model.ResultReject:
		detail["summary"] = "模拟运行：审核不通过"
		detail["risk_level"] = "high"
		detail["score"] = 0.92
		detail["reasons"] = []string{"模拟运行命中高风险规则"}
	}
	detail["result"] = state
	return detail
}
