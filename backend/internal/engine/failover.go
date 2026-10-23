package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"smartdoc-vision/backend/internal/ai"
	"smartdoc-vision/backend/internal/model"
)

const providerAttemptTimeout = 45 * time.Second

func reviewAttemptTimeout(kind string) time.Duration {
	if kind == model.ProviderJEV {
		return 60 * time.Second
	}
	return 30 * time.Second
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
	if err := q.Order("id asc").Find(&providers).Error; err != nil {
		return nil, err
	}

	available := make([]model.AIProvider, 0, len(providers))
	for _, provider := range providers {
		// A tripped breaker is only cleared by a successful health probe or a
		// manual connection test, never merely by the cooldown elapsing.
		if provider.BreakerUntil != nil {
			continue
		}
		available = append(available, provider)
	}
	sort.SliceStable(available, func(i, j int) bool {
		if available[i].ID == primary.ID {
			return true
		}
		if available[j].ID == primary.ID {
			return false
		}
		if preferReasoning {
			left := ai.ParseConfig(available[i])
			right := ai.ParseConfig(available[j])
			if left.SupportsThinking != right.SupportsThinking {
				return left.SupportsThinking
			}
		}
		return available[i].ID < available[j].ID
	})
	return available, nil
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
		adapter, adapterErr := ai.AdapterFor(candidate.Kind)
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
			e.clearProviderFailure(candidate)
			return verdict, candidate, nil
		}
		if reviewErr == nil {
			reviewErr = fmt.Errorf("模型未返回结论")
		}
		lastErr = reviewErr
		e.recordProviderFailure(candidate, reviewErr)
		log.Printf("task %s model %s(%s) failed, trying next provider: %v",
			task.TaskNo, candidate.Name, candidate.Model, reviewErr)
	}
	return nil, primary, lastErr
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
		adapter, err := ai.AdapterFor(provider.Kind)
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
