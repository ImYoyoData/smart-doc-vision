package engine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/media"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/rules"
	"smartdoc-vision/backend/internal/security"
)

type Engine struct {
	db      *gorm.DB
	redis   *redis.Client
	cipher  *security.Cipher
	cfg     config.Config
	fetcher *media.Fetcher
	queue   chan uint64
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	once    sync.Once
}

func New(db *gorm.DB, cache *redis.Client, cipher *security.Cipher, cfg config.Config) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		db:      db,
		redis:   cache,
		cipher:  cipher,
		cfg:     cfg,
		fetcher: media.NewFetcher(cfg.AllowPrivate),
		queue:   make(chan uint64, 2000),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (e *Engine) Start() {
	e.once.Do(func() {
		if err := e.recoverQueue(); err != nil {
			log.Printf("engine recover queue: %v", err)
		}
		for i := 0; i < e.cfg.WorkerCount; i++ {
			e.wg.Add(1)
			go e.worker(i + 1)
		}
		log.Printf("audit engine started with %d workers", e.cfg.WorkerCount)
	})
}

func (e *Engine) Stop() {
	e.cancel()
	e.wg.Wait()
}

func (e *Engine) Enqueue(ctx context.Context, taskID uint64) error {
	select {
	case e.queue <- taskID:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) recoverQueue() error {
	if err := e.db.Model(&model.Task{}).
		Where("status = ?", model.TaskProcessing).
		Updates(map[string]any{
			"status":        model.TaskQueued,
			"error_message": "服务重启，任务自动重新入队",
		}).Error; err != nil {
		return err
	}
	var ids []uint64
	if err := e.db.Model(&model.Task{}).
		Where("status = ?", model.TaskQueued).
		Order("id asc").Limit(5000).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		select {
		case e.queue <- id:
		default:
			return fmt.Errorf("queue full while recovering task %d", id)
		}
	}
	if len(ids) > 0 {
		log.Printf("requeued %d audit tasks", len(ids))
	}
	return nil
}

func (e *Engine) worker(n int) {
	defer e.wg.Done()
	for {
		select {
		case <-e.ctx.Done():
			return
		case id, ok := <-e.queue:
			if !ok {
				return
			}
			e.process(id)
		}
	}
}

func (e *Engine) process(taskID uint64) {
	ctx, cancel := context.WithTimeout(e.ctx, 2*time.Minute)
	defer cancel()

	now := time.Now()
	res := e.db.Model(&model.Task{}).
		Where("id = ? AND status = ?", taskID, model.TaskQueued).
		Updates(map[string]any{
			"status":     model.TaskProcessing,
			"started_at": now,
			"updated_at": now,
		})
	if res.Error != nil || res.RowsAffected == 0 {
		return
	}
	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return
	}
	e.event(task, "", model.TaskProcessing, "Worker 已领取任务", 0)

	var audit model.Audit
	if err := e.db.First(&audit, task.AuditID).Error; err != nil {
		e.fail(task, fmt.Errorf("审核配置不存在: %w", err))
		return
	}
	if audit.Status != model.AuditActive {
		e.fail(task, fmt.Errorf("审核配置当前状态为 %s，不能执行", audit.Status))
		return
	}
	var provider model.AIProvider
	if err := e.db.First(&provider, task.ProviderID).Error; err != nil {
		e.fail(task, fmt.Errorf("模型供应商不存在: %w", err))
		return
	}
	if provider.Status != model.ProviderActive {
		e.fail(task, fmt.Errorf("模型供应商 %s 已停用", provider.Name))
		return
	}
	apiKey, err := e.cipher.Decrypt(provider.APIKeyEnc)
	if err != nil {
		e.fail(task, fmt.Errorf("读取模型密钥失败: %w", err))
		return
	}
	var input model.TaskInput
	if err := model.DecodeJSON(task.InputJSON, &input); err != nil {
		e.fail(task, fmt.Errorf("任务输入解析失败: %w", err))
		return
	}
	adapter, err := ai.AdapterFor(provider.Kind)
	if err != nil {
		e.fail(task, err)
		return
	}

	started := time.Now()
	verdict, reviewErr := adapter.Review(ctx, ai.Request{
		Provider: provider,
		APIKey:   apiKey,
		Audit:    audit,
		Input:    input,
	})
	if verdict == nil {
		verdict = &model.Verdict{}
	}
	if verdict.Provider == "" {
		verdict.Provider = provider.Kind
	}
	if verdict.Model == "" {
		verdict.Model = provider.Model
	}
	if reviewErr != nil {
		log.Printf("task %s model review failed: %v", task.TaskNo, reviewErr)
		verdict.LatencyMS = time.Since(started).Milliseconds()
		fallback := normalizeResult(audit.OnModelFail)
		verdict.Result = fallback
		verdict.RiskLevel = "medium"
		verdict.Score = fallbackScore(fallback)
		verdict.Summary = "模型调用失败，已按审核配置执行兜底策略"
		verdict.Reasons = []string{reviewErr.Error()}
		verdict.Labels = []string{"模型异常兜底"}
	}
	verdict.Provider = provider.Kind
	if verdict.Model == "" {
		verdict.Model = provider.Model
	}
	if verdict.Provider == model.ProviderJEV && reviewErr != nil {
		verdict.Provider = model.ProviderJEV
	}
	if reviewErr == nil && strings.EqualFold(audit.DecisionMode, model.DecisionClassify) {
		applyClassificationMapping(audit, verdict)
	}

	ruleResult := rules.Evaluate(audit, rules.Context{
		Verdict:    verdict,
		Input:      input,
		SourceType: task.SourceType,
	})
	finalResult := normalizeResult(ruleResult.FinalResult)
	if finalResult == "" {
		finalResult = normalizeResult(verdict.Result)
	}
	if finalResult == "" {
		finalResult = normalizeResult(audit.DefaultResult)
	}
	if finalResult == "" {
		finalResult = model.ResultReview
	}

	routeResults, routeOverride := e.runRoutes(audit, task, finalResult, input, map[string]any{
		"summary": verdict.Summary, "reasons": verdict.Reasons, "labels": verdict.Labels,
	})
	if routeOverride != "" {
		finalResult = routeOverride
		verdict.Reasons = append(verdict.Reasons, "结果动作失败，已按路由策略转人工复核")
	}

	resultDetail := map[string]any{
		"summary":             verdict.Summary,
		"reasons":             verdict.Reasons,
		"labels":              verdict.Labels,
		"risk_level":          verdict.RiskLevel,
		"score":               verdict.Score,
		"provider":            verdict.Provider,
		"model":               verdict.Model,
		"latency_ms":          verdict.LatencyMS,
		"category":            verdict.Category,
		"category_name":       verdict.CategoryName,
		"category_confidence": verdict.CategoryConfidence,
		"rule":                ruleResult,
		"routes":              routeResults,
		"media_type":          task.MediaType,
		"callback":            nil,
	}
	if reviewErr != nil {
		resultDetail["model_error"] = reviewErr.Error()
	}
	modelJSON := model.JSONBytes(verdict)
	ruleJSON := model.JSONBytes(ruleResult)
	resultJSON := model.JSONBytes(resultDetail)

	status := model.TaskSuccess
	var finished *time.Time
	if finalResult == model.ResultReview {
		status = model.TaskReviewPending
	} else {
		finished = ptrTime(time.Now())
	}
	duration := time.Since(started).Milliseconds()
	if verdict.LatencyMS == 0 {
		verdict.LatencyMS = duration
	}
	errMsg := ""
	if reviewErr != nil {
		errMsg = truncate(reviewErr.Error(), 500)
	}
	updates := map[string]any{
		"status":        status,
		"result":        finalResult,
		"model_result":  modelJSON,
		"rule_trace":    ruleJSON,
		"result_json":   resultJSON,
		"duration_ms":   duration,
		"error_message": errMsg,
		"finished_at":   finished,
		"updated_at":    time.Now(),
	}
	if err := e.db.Model(&model.Task{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
		log.Printf("update task %d: %v", task.ID, err)
		return
	}
	message := fmt.Sprintf("审核完成，结论 %s（规则：%s）", finalResult, emptyDash(ruleResult.MatchedRule))
	if reviewErr != nil {
		message = fmt.Sprintf("模型异常，按兜底策略给出 %s", finalResult)
	}
	e.event(task, model.TaskProcessing, status, message, duration)
	e.writeAudit(task, "TASK_COMPLETE", map[string]any{
		"result": finalResult, "status": status, "provider": verdict.Provider, "model": verdict.Model,
	})
	if status == model.TaskSuccess {
		e.runCallback(task.ID)
	}
}

func (e *Engine) fail(task model.Task, err error) {
	msg := truncate(err.Error(), 500)
	now := time.Now()
	duration := int64(0)
	if task.StartedAt != nil {
		duration = now.Sub(*task.StartedAt).Milliseconds()
	}
	_ = e.db.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{
		"status":        model.TaskFailed,
		"result":        model.ResultUnknown,
		"error_message": msg,
		"duration_ms":   duration,
		"finished_at":   now,
		"updated_at":    now,
	}).Error
	e.event(task, model.TaskProcessing, model.TaskFailed, msg, duration)
	e.writeAudit(task, "TASK_FAILED", map[string]any{"error": msg})
}

func (e *Engine) event(task model.Task, from, to, message string, cost int64) {
	ev := model.TaskEvent{
		TaskID:     task.ID,
		ProjectID:  task.ProjectID,
		FromStatus: from,
		ToStatus:   to,
		Message:    truncate(message, 500),
		CostMS:     cost,
	}
	if err := e.db.Create(&ev).Error; err != nil {
		log.Printf("write task event: %v", err)
	}
}

func (e *Engine) writeAudit(task model.Task, action string, detail map[string]any) {
	pid := task.ProjectID
	log := model.AuditLog{
		ProjectID:  &pid,
		Operator:   "system",
		OperatorIP: "",
		Action:     action,
		ObjectType: "TASK",
		ObjectID:   task.TaskNo,
		Result:     "SUCCESS",
		DetailJSON: model.JSONBytes(detail),
	}
	_ = e.db.Create(&log).Error
}

// RunCallback loads a task and dispatches its configured business callback.
// It is used after human review turns a REVIEW_PENDING task into a final result.
func (e *Engine) RunCallback(taskID uint64) {
	go e.runCallback(taskID)
}

func (e *Engine) runCallback(taskID uint64) {
	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return
	}
	if task.Status != model.TaskSuccess {
		return
	}
	var input model.TaskInput
	_ = model.DecodeJSON(task.InputJSON, &input)
	var audit model.Audit
	_ = e.db.First(&audit, task.AuditID).Error
	var project model.Project
	_ = e.db.First(&project, task.ProjectID).Error

	targetURL := strings.TrimSpace(input.CallbackURL)
	auditCallback := false
	if audit.CallbackEnabled && audit.CallbackURL != "" {
		auditCallback = true
		var allowed []string
		_ = model.DecodeJSON(audit.CallbackResults, &allowed)
		if len(allowed) > 0 && !containsResult(allowed, task.Result) {
			return
		}
		if targetURL == "" {
			targetURL = audit.CallbackURL
		}
	}
	if targetURL == "" {
		return
	}
	if !e.cfg.AllowPrivate {
		if err := media.ValidateOutboundURL(targetURL); err != nil {
			e.appendCallbackResult(task, map[string]any{
				"ok": false, "target_url": targetURL, "error": "回调地址未通过 SSRF 校验: " + err.Error(),
			})
			return
		}
	}

	var detail map[string]any
	_ = model.DecodeJSON(task.ResultJSON, &detail)
	if detail == nil {
		detail = map[string]any{}
	}
	mediaPayload := make([]map[string]any, 0, len(input.Media))
	for _, item := range input.Media {
		entry := map[string]any{"type": item.Type, "name": item.Name}
		if item.URL != "" {
			entry["url"] = item.URL
		}
		if item.Content != "" && (audit.CallbackIncludeContent || !auditCallback) {
			entry["content"] = item.Content
		}
		mediaPayload = append(mediaPayload, entry)
	}
	payload := map[string]any{
		"event":               "audit.completed",
		"task_id":             task.ID,
		"task_no":             task.TaskNo,
		"project_id":          task.ProjectID,
		"project_code":        project.Code,
		"project_name":        project.Name,
		"audit_id":            task.AuditID,
		"audit_code":          audit.Code,
		"audit_name":          audit.Name,
		"source_type":         task.SourceType,
		"biz_key":             task.BizKey,
		"status":              task.Status,
		"result":              task.Result,
		"summary":             detail["summary"],
		"reasons":             detail["reasons"],
		"labels":              detail["labels"],
		"risk_level":          detail["risk_level"],
		"score":               detail["score"],
		"category":            detail["category"],
		"category_name":       detail["category_name"],
		"category_confidence": detail["category_confidence"],
		"rule":                detail["rule"],
		"provider":            detail["provider"],
		"model":               detail["model"],
		"media":               mediaPayload,
		"ext":                 input.Ext,
		"result_detail":       detail,
		"finished_at":         task.FinishedAt,
	}
	body, _ := json.Marshal(payload)

	secret := ""
	if audit.CallbackSecretEnc != "" {
		if decrypted, err := e.cipher.Decrypt(audit.CallbackSecretEnc); err == nil {
			secret = decrypted
		}
	}
	var customHeaders map[string]string
	_ = model.DecodeJSON(audit.CallbackHeaders, &customHeaders)

	client := &http.Client{Timeout: 10 * time.Second}
	var lastErr string
	var lastStatus int
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(e.ctx, http.MethodPost, targetURL, bytes.NewReader(body))
		if err != nil {
			lastErr = err.Error()
			break
		}
		for k, v := range customHeaders {
			if strings.TrimSpace(k) != "" {
				req.Header.Set(k, v)
			}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "SmartDoc-Vision-Webhook/1.0")
		req.Header.Set("X-Idempotency-Key", task.TaskNo)
		req.Header.Set("X-SmartDoc-Event", "audit.completed")
		req.Header.Set("X-SmartDoc-Task", task.TaskNo)
		req.Header.Set("X-SmartDoc-Result", task.Result)
		req.Header.Set("X-SmartDoc-Timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
		if secret != "" {
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(body)
			req.Header.Set("X-SmartDoc-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
		} else {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			lastStatus = resp.StatusCode
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				e.appendCallbackResult(task, map[string]any{
					"ok": true, "target_url": targetURL, "http_status": resp.StatusCode,
					"attempts": attempt, "signed": secret != "",
					"response": truncate(string(raw), 300), "result": task.Result,
				})
				return
			}
			lastErr = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
		}
		if attempt < 3 {
			select {
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			case <-e.ctx.Done():
				return
			}
		}
	}
	e.appendCallbackResult(task, map[string]any{
		"ok": false, "target_url": targetURL, "http_status": lastStatus,
		"error": truncate(lastErr, 400), "attempts": 3, "signed": secret != "", "result": task.Result,
	})
	e.writeAudit(task, "TASK_CALLBACK_FAILED", map[string]any{"target_url": targetURL, "error": truncate(lastErr, 300)})
}

func applyClassificationMapping(audit model.Audit, verdict *model.Verdict) {
	categories := ai.AuditCategories(audit)
	if len(categories) == 0 {
		verdict.Result = model.ResultReview
		verdict.Reasons = append(verdict.Reasons, "审核配置缺少分类列表，已转人工复核")
		return
	}
	code := strings.ToLower(strings.TrimSpace(verdict.Category))
	if code == "" {
		verdict.Result = model.ResultReview
		verdict.Reasons = append(verdict.Reasons, "模型未返回分类编码，已转人工复核")
		return
	}
	category, found := ai.FindCategory(categories, code)
	if !found {
		verdict.Category = code
		verdict.CategoryName = "未知分类"
		verdict.Result = model.ResultReview
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("分类编码 %s 不在配置中，已转人工复核", code))
		return
	}
	verdict.Category = category.Code
	verdict.CategoryName = category.Name
	if category.Result == model.ResultPass || category.Result == model.ResultReject || category.Result == model.ResultReview {
		verdict.Result = category.Result
	} else {
		verdict.Result = model.ResultReview
	}
	if !containsText(verdict.Labels, "分类:"+category.Name) {
		verdict.Labels = append(verdict.Labels, "分类:"+category.Name)
	}
	verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("分类映射：%s → %s", category.Name, verdict.Result))
}

func containsText(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsResult(allowed []string, result string) bool {
	for _, v := range allowed {
		if strings.EqualFold(v, result) {
			return true
		}
	}
	return false
}

func (e *Engine) appendCallbackResult(task model.Task, callback any) {
	var detail map[string]any
	_ = model.DecodeJSON(task.ResultJSON, &detail)
	if detail == nil {
		detail = map[string]any{}
	}
	detail["callback"] = callback
	_ = e.db.Model(&model.Task{}).Where("id = ?", task.ID).
		Update("result_json", model.JSONBytes(detail)).Error
}

func normalizeResult(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case model.ResultPass:
		return model.ResultPass
	case model.ResultReject:
		return model.ResultReject
	case model.ResultReview:
		return model.ResultReview
	default:
		return ""
	}
}

func fallbackScore(result string) float64 {
	switch result {
	case model.ResultPass:
		return 0.2
	case model.ResultReject:
		return 0.9
	default:
		return 0.5
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func emptyDash(s string) string {
	if s == "" {
		return "默认策略"
	}
	return s
}

var ErrQueueClosed = errors.New("engine queue closed")
