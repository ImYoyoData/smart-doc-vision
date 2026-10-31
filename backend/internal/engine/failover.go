package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/usage"
)

const providerAttemptTimeout = 45 * time.Second

func reviewAttemptTimeout(kind string) time.Duration {
	if kind == model.ProviderJEV {
		return 60 * time.Second
	}
	// 通用模型带图审核（vision）真实响应常在 30s 以上，给足单次尝试时间。
	return 45 * time.Second
}

// taskScopedFailure reports whether a review error is caused by the task's own
// content — an unfetchable image or an upstream 4xx about the request payload.
// Another provider cannot fix such a failure (the same content fails the same
// way), so it must not trip the provider breaker and bench healthy models.
func taskScopedFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "张图片失败") {
		return true
	}
	if strings.Contains(msg, "Failed to download image") {
		return true
	}
	return false
}

// providerCandidates returns the models that may serve this audit, in the order
// they should be tried. The audit's bound model is first when it is healthy;
// other active models of the same business category follow. Cooling models are
// kept as a last resort, ordered by the earliest recovery time.
func (e *Engine) providerCandidates(primary model.AIProvider, preferReasoning bool) ([]model.AIProvider, error) {
	category := providerCategory(primary.Kind)
	q := e.db.Where("project_id = ? AND status = ?", primary.ProjectID, model.ProviderActive)
	if category == model.ProviderJEV {
		q = q.Where("kind = ?", model.ProviderJEV)
	} else {
		q = q.Where("kind IN ?", []string{model.ProviderOpenAI, model.ProviderAnthropic})
	}
	var providers []model.AIProvider
	if err := q.Order("priority asc, id asc").Find(&providers).Error; err != nil {
		return nil, err
	}

	available := make([]model.AIProvider, 0, len(providers))
	cooling := make([]string, 0)
	for _, provider := range providers {
		// A tripped breaker is only cleared by a successful health probe or a
		// manual connection test, never merely by the cooldown elapsing.
		if provider.BreakerUntil != nil {
			reason := provider.BreakerReason
			if len([]rune(reason)) > 60 {
				reason = string([]rune(reason)[:60]) + "…"
			}
			cooling = append(cooling, fmt.Sprintf("%s（冷却至 %s：%s）",
				provider.Name, provider.BreakerUntil.Local().Format("15:04:05"), reason))
			continue
		}
		available = append(available, provider)
	}
	if len(available) == 0 && len(cooling) > 0 {
		return nil, fmt.Errorf("项目内没有可用的同类模型，正在熔断冷却：%s；可在「模型接入」执行测试连接提前恢复", strings.Join(cooling, "；"))
	}
	return orderProviderCandidates(available, primary.ID, preferReasoning), nil
}

// orderProviderCandidates keeps the audit-bound provider first, then orders the
// fallbacks by explicit priority (smaller first). Reasoning preference and ID
// only break ties.
func orderProviderCandidates(available []model.AIProvider, primaryID uint64, preferReasoning bool) []model.AIProvider {
	out := append([]model.AIProvider(nil), available...)
	sort.SliceStable(out, func(i, j int) bool {
		iPrimary := out[i].ID == primaryID
		jPrimary := out[j].ID == primaryID
		if iPrimary != jPrimary {
			return iPrimary
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		if preferReasoning {
			left := ai.ParseConfig(out[i])
			right := ai.ParseConfig(out[j])
			if left.SupportsThinking != right.SupportsThinking {
				return left.SupportsThinking
			}
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func providerCategory(kind string) string {
	if kind == model.ProviderJEV {
		return model.ProviderJEV
	}
	return "GENERAL"
}

// reviewWithFailover tries the bound provider first and immediately moves to
// the next compatible model when a call fails. Failed providers enter a
// cooldown so following tasks skip them until the recovery loop clears them.
func (e *Engine) reviewWithFailover(
	ctx context.Context,
	task model.Task,
	audit model.Audit,
	input model.TaskInput,
	primary model.AIProvider,
) (*model.Verdict, model.AIProvider, error) {
	preferReasoning := ai.ParseConfig(primary).PreferReasoning
	candidates, err := e.providerCandidates(primary, preferReasoning)
	if err != nil {
		return nil, primary, err
	}
	if len(candidates) == 0 {
		return nil, primary, fmt.Errorf("项目内没有可用的同类模型（可能正在熔断冷却）")
	}

	var lastErr error
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			return nil, candidate, ctx.Err()
		default:
		}
		apiKey, decryptErr := e.cipher.Decrypt(candidate.APIKeyEnc)
		if decryptErr != nil {
			lastErr = fmt.Errorf("读取模型 %s 密钥失败: %w", candidate.Name, decryptErr)
			e.recordProviderFailure(candidate, lastErr)
			continue
		}
		adapter, adapterErr := ai.AdapterFor(candidate.Kind, e.loader)
		if adapterErr != nil {
			lastErr = adapterErr
			e.recordProviderFailure(candidate, adapterErr)
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, reviewAttemptTimeout(candidate.Kind))
		started := time.Now()
		verdict, reviewErr := adapter.Review(attemptCtx, ai.Request{
			Provider: candidate,
			APIKey:   apiKey,
			Audit:    audit,
			Input:    input,
		})
		cancel()
		if reviewErr == nil && verdict != nil {
			if verdict.LatencyMS == 0 {
				verdict.LatencyMS = time.Since(started).Milliseconds()
			}
			e.recordReviewUsage(candidate, task, audit, verdict, started, nil)
			e.clearProviderFailure(candidate)
			return verdict, candidate, nil
		}
		if reviewErr == nil {
			reviewErr = fmt.Errorf("模型未返回结论")
		}
		lastErr = reviewErr
		e.recordReviewUsage(candidate, task, audit, nil, started, reviewErr)
		if taskScopedFailure(reviewErr) {
			// 任务内容自身的问题（图片拉不到、上游拒收请求体）：换模型也会
			// 同样失败，直接终止并走任务兜底，不记熔断以免连坐健康模型。
			log.Printf("task %s content-scoped failure on %s(%s), skip fallback: %v",
				task.TaskNo, candidate.Name, candidate.Model, reviewErr)
			return nil, candidate, reviewErr
		}
		e.recordProviderFailure(candidate, reviewErr)
		log.Printf("task %s model %s(%s) failed, trying next provider: %v",
			task.TaskNo, candidate.Name, candidate.Model, reviewErr)
	}
	return nil, primary, lastErr
}

// recordReviewUsage persists one review attempt. Failures are logged without
// tokens so the provider card's request count still reflects real traffic.
func (e *Engine) recordReviewUsage(
	provider model.AIProvider,
	task model.Task,
	audit model.Audit,
	verdict *model.Verdict,
	started time.Time,
	callErr error,
) {
	entry := usage.Entry{
		ProjectID:    provider.ProjectID,
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		Kind:         provider.Kind,
		Model:        provider.Model,
		Source:       model.UsageSourceReview,
		AuditID:      audit.ID,
		TaskID:       task.ID,
		DurationMS:   time.Since(started).Milliseconds(),
	}
	if callErr != nil {
		entry.Status = model.UsageStatusError
		entry.ErrorMessage = callErr.Error()
	} else if verdict != nil {
		if verdict.Model != "" {
			entry.Model = verdict.Model
		}
		entry.InputTokens = verdict.InputTokens
		entry.CachedTokens = verdict.CachedTokens
		entry.OutputTokens = verdict.OutputTokens
		entry.TotalTokens = verdict.TotalTokens
	}
	usage.Record(e.db, entry)
}

func (e *Engine) recordProviderFailure(provider model.AIProvider, err error) {
	failures := provider.FailureCount + 1
	cooldown := breakerCooldown(failures)
	until := time.Now().Add(cooldown)
	reason := err.Error()
	if len([]rune(reason)) > 500 {
		reason = string([]rune(reason)[:500])
	}
	_ = e.db.Model(&model.AIProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"breaker_until":  until,
		"breaker_reason": reason,
		"failure_count":  failures,
	}).Error
}

func (e *Engine) clearProviderFailure(provider model.AIProvider) {
	if provider.FailureCount == 0 && provider.BreakerUntil == nil {
		return
	}
	_ = e.db.Model(&model.AIProvider{}).Where("id = ?", provider.ID).Updates(map[string]any{
		"breaker_until":  nil,
		"breaker_reason": "",
		"failure_count":  0,
	}).Error
}

func breakerCooldown(failures int) time.Duration {
	cooldown := 30 * time.Second
	for i := 1; i < failures; i++ {
		cooldown *= 2
		if cooldown >= 10*time.Minute {
			return 10 * time.Minute
		}
	}
	return cooldown
}

func (e *Engine) breakerRecovery() {
	defer e.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.recoverBreakers()
		}
	}
}

func (e *Engine) recoverBreakers() {
	var providers []model.AIProvider
	if err := e.db.Where("status = ? AND breaker_until IS NOT NULL AND breaker_until <= ?",
		model.ProviderActive, time.Now()).Find(&providers).Error; err != nil {
		log.Printf("breaker recovery query: %v", err)
		return
	}
	for _, provider := range providers {
		select {
		case <-e.ctx.Done():
			return
		default:
		}
		apiKey, err := e.cipher.Decrypt(provider.APIKeyEnc)
		if err != nil {
			e.recordProviderFailure(provider, err)
			continue
		}
		adapter, err := ai.AdapterFor(provider.Kind, e.loader)
		if err != nil {
			e.recordProviderFailure(provider, err)
			continue
		}
		ctx, cancel := context.WithTimeout(e.ctx, providerAttemptTimeout)
		testProvider := ai.WithRandomSessionHeader(provider)
		result, testErr := adapter.Test(ctx, ai.Request{Provider: testProvider, APIKey: apiKey})
		cancel()
		if testErr == nil && result.OK {
			e.clearProviderFailure(provider)
			log.Printf("provider %s recovered, breaker cleared", provider.Name)
			continue
		}
		reason := result.Message
		if testErr != nil {
			reason = testErr.Error()
		}
		if reason == "" {
			reason = "恢复探测失败"
		}
		e.recordProviderFailure(provider, fmt.Errorf("%s", reason))
	}
}
