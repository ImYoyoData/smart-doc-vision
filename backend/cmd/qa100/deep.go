package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

func (r *runner) runDeepChecks(ctx context.Context) error {
	suffix := time.Now().Format("150405")
	checks := []struct {
		name string
		fn   func(context.Context, string) error
	}{
		{"project-zero-defaults", r.checkProjectZeroDefaults},
		{"pure-classifier", r.checkPureClassifier},
		{"concurrent-dedupe", r.checkConcurrentDedupe},
		{"dedupe-window-boundary", r.checkDedupeWindowBoundary},
		{"provider-failover", r.checkProviderFailover},
		{"callback-retry", r.checkCallbackRetry},
	}
	var failures []string
	for _, check := range checks {
		started := time.Now()
		if err := check.fn(ctx, suffix); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", check.name, err))
			fmt.Printf("DEEP %-20s FAIL %.1fs %v\n", check.name, time.Since(started).Seconds(), err)
			continue
		}
		fmt.Printf("DEEP %-20s OK   %.1fs\n", check.name, time.Since(started).Seconds())
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d deep checks failed: %s", len(failures), strings.Join(failures, " | "))
	}
	fmt.Println("DEEP ALL OK")
	return nil
}

func (r *runner) checkProjectZeroDefaults(ctx context.Context, suffix string) error {
	code := fmt.Sprintf("qa100-%s-z%s", r.state.RunID, suffix)
	p, err := r.client.createProjectWithOptions(ctx, "QA100 zero defaults", code, map[string]any{
		"openapi_rate_limit": 0, "js_max_http_requests": 0,
	})
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	defer func() {
		if cleanupErr := r.deleteTemporaryProject(context.Background(), p.ID); cleanupErr != nil {
			fmt.Printf("DEEP cleanup temporary project %d: %v\n", p.ID, cleanupErr)
		}
	}()
	var rateLimit, httpRequests int
	if err := r.db.QueryRowContext(ctx, "SELECT open_api_rate_limit, js_max_http_requests FROM projects WHERE id = ?", p.ID).
		Scan(&rateLimit, &httpRequests); err != nil {
		return err
	}
	if rateLimit != 0 {
		return fmt.Errorf("openapi_rate_limit=%d want 0 (0 means unlimited)", rateLimit)
	}
	if httpRequests != 0 {
		return fmt.Errorf("js_max_http_requests=%d want 0", httpRequests)
	}
	return nil
}

func (r *runner) deleteTemporaryProject(ctx context.Context, projectID uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	tables := []string{
		"script_executions", "task_events", "tasks", "trigger_sources",
		"redis_connections", "ai_providers", "provider_usage_logs", "audit_logs",
		"api_credentials", "project_members", "project_secrets", "audits",
	}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s` WHERE project_id = ?", table), projectID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", projectID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (r *runner) checkPureClassifier(ctx context.Context, suffix string) error {
	code := fmt.Sprintf("qa100_%s_dp_%s", r.state.RunID, suffix)
	categories := []map[string]any{
		{"code": "safe", "name": "安全", "description": "低风险", "result": "PASS"},
		{"code": "manual", "name": "待定", "description": "需人工确认", "result": "REVIEW"},
	}
	auditID, err := r.createRawAudit(ctx, code, "QA100 deep pure classifier", "CLASSIFY_ONLY", "", r.state.ProviderID,
		map[string]any{"categories": categories})
	if err != nil {
		return err
	}
	for index, marker := range []string{"QA_CLASSIFY_REVIEW", "QA_UNKNOWN"} {
		task, _, err := r.client.submitTask(ctx, r.state.ProjectID, auditID,
			fmt.Sprintf("qa100:%s:deep:pure:%s:%d", r.state.RunID, suffix, index),
			marker+" deep pure classifier", map[string]any{"deep": "pure", "case": marker}, nil)
		if err != nil {
			return fmt.Errorf("submit %s: %w", marker, err)
		}
		final, err := r.waitTerminal(ctx, task.ID)
		if err != nil {
			return fmt.Errorf("%s: %w", marker, err)
		}
		if final.Status != "SUCCESS" || final.Result != "PASS" {
			return fmt.Errorf("%s status=%s result=%s want SUCCESS/PASS", marker, final.Status, final.Result)
		}
		if _, reviewed := final.ResultDetail["manual_review"]; reviewed {
			return fmt.Errorf("%s unexpectedly required manual review", marker)
		}
	}
	var rawCategories string
	if err := r.db.QueryRowContext(ctx, "SELECT categories FROM audits WHERE id = ?", auditID).Scan(&rawCategories); err != nil {
		return fmt.Errorf("query categories: %w", err)
	}
	if !strings.Contains(rawCategories, `"unknown"`) {
		return fmt.Errorf("pure classifier categories missing unknown: %s", rawCategories)
	}
	var stored []map[string]any
	if err := json.Unmarshal([]byte(rawCategories), &stored); err != nil {
		return fmt.Errorf("decode categories: %w", err)
	}
	stored = append(stored, map[string]any{"code": "extra", "name": "补充", "description": "deep check", "result": "PASS"})
	if err := r.client.updateAudit(ctx, auditID, map[string]any{"name": "QA100 deep pure classifier", "categories": stored}); err != nil {
		return fmt.Errorf("edit unknown/category list: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, "SELECT categories FROM audits WHERE id = ?", auditID).Scan(&rawCategories); err != nil {
		return fmt.Errorf("re-query categories: %w", err)
	}
	if !strings.Contains(rawCategories, `"extra"`) {
		return fmt.Errorf("category edit not persisted: %s", rawCategories)
	}
	return nil
}

func (r *runner) checkConcurrentDedupe(ctx context.Context, suffix string) error {
	code := fmt.Sprintf("qa100_%s_dd_%s", r.state.RunID, suffix)
	round := 900
	cfg := &configSpec{
		Key: "dd", Code: code, Name: "QA100 deep concurrent dedupe",
		Mode: "REVIEW", Strictness: "standard", CallbackKind: "http",
		CallbackSecret: fmt.Sprintf("qa100-secret-%s-dd", r.state.RunID),
		IncludeContent: true, DedupeWindow: 120,
	}
	if err := r.createAudit(ctx, round, cfg); err != nil {
		return err
	}
	bizKey := fmt.Sprintf("qa100:%s:deep:dedupe:%s", r.state.RunID, suffix)
	text := "QA_PASS deep concurrent dedupe " + suffix
	ext := map[string]any{"deep": "dedupe"}
	type submitResult struct {
		task       *task
		duplicated bool
		err        error
	}
	results := make([]submitResult, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			t, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, cfg.AuditID, bizKey, text, ext, nil)
			results[i] = submitResult{task: t, duplicated: duplicated, err: err}
		}()
	}
	wg.Wait()
	for i, result := range results {
		if result.err != nil {
			return fmt.Errorf("submit %d: %w", i, result.err)
		}
	}
	if results[0].task == nil || results[1].task == nil {
		return fmt.Errorf("nil task response")
	}
	if results[0].task.ID != results[1].task.ID {
		return fmt.Errorf("concurrent duplicate created different tasks %d and %d", results[0].task.ID, results[1].task.ID)
	}
	if results[0].duplicated == results[1].duplicated {
		return fmt.Errorf("duplicate flags both %v; one request should be marked duplicated", results[0].duplicated)
	}
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE biz_key = ?", bizKey).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("dedupe stored %d tasks want 1", count)
	}
	c := &caseSpec{
		Round: round, ConfigKey: "dd", AuditID: cfg.AuditID, Secret: cfg.CallbackSecret,
		Kind: "CONCURRENT_DEDUPE", Expected: "PASS", Task: results[0].task,
		TaskID: results[0].task.ID, TaskNo: results[0].task.TaskNo,
	}
	r.waitCase(ctx, c)
	r.verifyCase(ctx, c)
	r.cleanupRoundRedis(ctx, round)
	if len(c.Failures) > 0 {
		return fmt.Errorf("%s", strings.Join(c.Failures, "; "))
	}
	return nil
}

func (r *runner) checkDedupeWindowBoundary(ctx context.Context, suffix string) error {
	code := fmt.Sprintf("qa100_%s_db_%s", r.state.RunID, suffix)
	auditID, err := r.createRawAudit(ctx, code, "QA100 deep dedupe boundary", "REVIEW", "standard", r.state.ProviderID,
		map[string]any{"dedupe_window_sec": 2})
	if err != nil {
		return err
	}
	// Submit just before a 2s bucket boundary, then submit the same payload just
	// after it. The previous-bucket lookup must still classify it as duplicate.
	next := time.Now().Truncate(2 * time.Second).Add(2 * time.Second)
	time.Sleep(time.Until(next.Add(-400 * time.Millisecond)))
	bizKey := fmt.Sprintf("qa100:%s:deep:boundary:%s", r.state.RunID, suffix)
	text := "QA_PASS dedupe window boundary " + suffix
	first, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, auditID, bizKey, text, map[string]any{"deep": "boundary"}, nil)
	if err != nil {
		return fmt.Errorf("first submit: %w", err)
	}
	if duplicated {
		return fmt.Errorf("first submit unexpectedly duplicated")
	}
	if _, err := r.waitTerminal(ctx, first.ID); err != nil {
		return fmt.Errorf("first task: %w", err)
	}
	time.Sleep(time.Until(next.Add(150 * time.Millisecond)))
	second, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, auditID, bizKey, text, map[string]any{"deep": "boundary"}, nil)
	if err != nil {
		return fmt.Errorf("second submit: %w", err)
	}
	if !duplicated {
		return fmt.Errorf("identical request across the window boundary was not deduped")
	}
	if second.ID != first.ID {
		return fmt.Errorf("boundary duplicate created new task %d vs %d", second.ID, first.ID)
	}
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE biz_key = ?", bizKey).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("stored %d tasks want 1", count)
	}
	return nil
}

func (r *runner) checkProviderFailover(ctx context.Context, suffix string) error {
	badProvider, err := r.client.createProviderWithConfig(ctx, r.state.ProjectID,
		"QA100 Fail "+suffix, "http://"+r.env.mockAddr+"/v1", "qa-fail-model",
		map[string]any{
			"supports_vision": false, "max_tokens": 512, "temperature": 0.1, "max_images": 1,
			"headers": map[string]string{"X-QA-Fail": "1"},
		}, 1)
	if err != nil {
		return fmt.Errorf("create failing provider: %w", err)
	}
	code := fmt.Sprintf("qa100_%s_df_%s", r.state.RunID, suffix)
	auditID, err := r.createRawAudit(ctx, code, "QA100 deep provider failover", "REVIEW", "standard", badProvider.ID, nil)
	if err != nil {
		return err
	}
	first, _, err := r.client.submitTask(ctx, r.state.ProjectID, auditID,
		fmt.Sprintf("qa100:%s:deep:failover:first:%s", r.state.RunID, suffix),
		"QA_PASS deep provider failover", map[string]any{"deep": "failover", "attempt": 1}, nil)
	if err != nil {
		return fmt.Errorf("first submit: %w", err)
	}
	final, err := r.waitTerminal(ctx, first.ID)
	if err != nil {
		return fmt.Errorf("first task: %w", err)
	}
	if final.Status != "SUCCESS" || final.Result != "PASS" {
		return fmt.Errorf("first task status=%s result=%s want SUCCESS/PASS", final.Status, final.Result)
	}
	badUsage, goodUsage, err := r.usageByTask(ctx, first.ID)
	if err != nil {
		return err
	}
	if badUsage["ERROR"] == 0 {
		return fmt.Errorf("failing primary provider has no ERROR usage record: %v", badUsage)
	}
	if goodUsage["SUCCESS"] == 0 {
		return fmt.Errorf("fallback provider has no SUCCESS usage record: %v", goodUsage)
	}
	var breakerUntil sql.NullTime
	var failureCount int
	if err := r.db.QueryRowContext(ctx, "SELECT breaker_until, failure_count FROM ai_providers WHERE id = ?", badProvider.ID).
		Scan(&breakerUntil, &failureCount); err != nil {
		return fmt.Errorf("query breaker: %w", err)
	}
	if !breakerUntil.Valid || failureCount == 0 {
		return fmt.Errorf("failing provider breaker not tripped (until=%v count=%d)", breakerUntil, failureCount)
	}
	second, _, err := r.client.submitTask(ctx, r.state.ProjectID, auditID,
		fmt.Sprintf("qa100:%s:deep:failover:second:%s", r.state.RunID, suffix),
		"QA_PASS deep provider failover", map[string]any{"deep": "failover", "attempt": 2}, nil)
	if err != nil {
		return fmt.Errorf("second submit: %w", err)
	}
	final2, err := r.waitTerminal(ctx, second.ID)
	if err != nil {
		return fmt.Errorf("second task: %w", err)
	}
	if final2.Status != "SUCCESS" || final2.Result != "PASS" {
		return fmt.Errorf("second task status=%s result=%s want SUCCESS/PASS", final2.Status, final2.Result)
	}
	badUsage2, goodUsage2, err := r.usageByTask(ctx, second.ID)
	if err != nil {
		return err
	}
	if badUsage2["ERROR"] != 0 {
		return fmt.Errorf("breaker did not skip failing provider: %v", badUsage2)
	}
	if goodUsage2["SUCCESS"] == 0 {
		return fmt.Errorf("fallback provider not used after breaker: %v", goodUsage2)
	}
	return nil
}

func (r *runner) checkCallbackRetry(ctx context.Context, suffix string) error {
	code := fmt.Sprintf("qa100_%s_dr_%s", r.state.RunID, suffix)
	secret := fmt.Sprintf("qa100-secret-%s-retry", r.state.RunID)
	auditID, err := r.createRawAudit(ctx, code, "QA100 deep callback retry", "REVIEW", "standard", r.state.ProviderID,
		map[string]any{
			"callback_enabled":         true,
			"callback_url":             "http://" + r.env.actionAddr + "/callback/flaky",
			"callback_secret":          secret,
			"callback_results":         []string{"PASS"},
			"callback_headers":         map[string]string{"X-QA-Retry": "1"},
			"callback_include_content": true,
		})
	if err != nil {
		return err
	}
	task, _, err := r.client.submitTask(ctx, r.state.ProjectID, auditID,
		fmt.Sprintf("qa100:%s:deep:retry:%s", r.state.RunID, suffix),
		"QA_PASS deep callback retry", map[string]any{"deep": "retry"}, nil)
	if err != nil {
		return err
	}
	final, err := r.waitTerminal(ctx, task.ID)
	if err != nil {
		return err
	}
	if final.Status != "SUCCESS" || final.Result != "PASS" {
		return fmt.Errorf("status=%s result=%s want SUCCESS/PASS", final.Status, final.Result)
	}
	records, err := r.actions.rec.wait("http-callback", task.TaskNo, 3, 30*time.Second)
	if err != nil {
		return fmt.Errorf("callback attempts: %w", err)
	}
	if len(records) < 3 {
		return fmt.Errorf("callback attempts=%d want at least 3", len(records))
	}
	for index, record := range records[:3] {
		if record.Result != "PASS" {
			return fmt.Errorf("callback attempt %d result=%s", index+1, record.Result)
		}
		if record.Headers.Get("X-QA-Retry") != "1" {
			return fmt.Errorf("callback attempt %d missing custom header", index+1)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		latest, _, err := r.client.getTask(ctx, task.ID)
		if err == nil {
			if cb, ok := latest.ResultDetail["callback"].(map[string]any); ok {
				if fmt.Sprint(cb["ok"]) == "true" && fmt.Sprint(cb["attempts"]) == "3" {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("callback result_detail did not report ok/attempts=3: %v", latest)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (r *runner) createRawAudit(ctx context.Context, code, name, mode, strictness string, providerID uint64, extra map[string]any) (uint64, error) {
	payload := map[string]any{
		"project_id": r.state.ProjectID, "name": name, "code": code,
		"description": "QA100 deep check", "media_types": []string{"text"},
		"decision_mode": mode, "provider_id": providerID,
		"review_prompt":  "QA100 deep check. Return the requested strict JSON verdict.",
		"default_result": "REVIEW", "on_model_fail": "REVIEW",
		"dedupe_window_sec": 0, "status": "ACTIVE",
	}
	if strictness != "" {
		payload["strictness"] = strictness
	}
	for key, value := range extra {
		payload[key] = value
	}
	audit, err := r.client.createAudit(ctx, payload)
	if err != nil {
		return 0, err
	}
	return audit.ID, nil
}

func (r *runner) waitTerminal(ctx context.Context, taskID uint64) (*task, error) {
	deadline := time.Now().Add(45 * time.Second)
	for {
		t, _, err := r.client.getTask(ctx, taskID)
		if err == nil {
			switch t.Status {
			case "SUCCESS", "FAILED", "REVIEW_PENDING", "STOPPED":
				return t, nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return nil, fmt.Errorf("timeout: %w", err)
			}
			return nil, fmt.Errorf("timeout waiting task %d", taskID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (r *runner) usageByTask(ctx context.Context, taskID uint64) (map[string]int, map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT provider_id, status FROM provider_usage_logs WHERE task_id = ? ORDER BY id", taskID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	bad := map[string]int{}
	good := map[string]int{}
	for rows.Next() {
		var providerID uint64
		var status string
		if err := rows.Scan(&providerID, &status); err != nil {
			return nil, nil, err
		}
		if providerID == r.state.ProviderID {
			good[status]++
		} else {
			bad[status]++
		}
	}
	return bad, good, rows.Err()
}
