package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

func (r *runner) recordSubmitLatency(d time.Duration) {
	r.latencyMu.Lock()
	r.submitLatencies = append(r.submitLatencies, d)
	r.latencyMu.Unlock()
}

func (r *runner) recordTaskLatency(d time.Duration) {
	r.latencyMu.Lock()
	r.taskLatencies = append(r.taskLatencies, d)
	r.latencyMu.Unlock()
}

func (r *runner) printLatencySummary() {
	r.latencyMu.Lock()
	submit := append([]time.Duration(nil), r.submitLatencies...)
	tasks := append([]time.Duration(nil), r.taskLatencies...)
	r.latencyMu.Unlock()
	printSeries := func(label string, values []time.Duration) {
		if len(values) == 0 {
			return
		}
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		fmt.Printf("%s p50=%s p95=%s max=%s n=%d\n",
			label, percentile(values, 0.50), percentile(values, 0.95), values[len(values)-1], len(values))
	}
	printSeries("LATENCY submit", submit)
	printSeries("LATENCY task", tasks)
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func (r *runner) runPairCheck(ctx context.Context, round int) ([]string, []string) {
	var failures, warnings []string
	first := round - 1
	if first < 1 {
		first = 1
	}
	expected := r.baselineTasks + r.expectedTasks
	var taskCount, stuck, reviewPending, unknownResult, failedRoutes, missingCallback, failedScripts, missingSuccessEvent, duplicateBizKeys, failedAuditLogs int
	queries := []struct {
		name  string
		query string
		dest  *int
	}{
		{"tasks", "SELECT COUNT(*) FROM tasks WHERE project_id = ?", &taskCount},
		{"stuck", "SELECT COUNT(*) FROM tasks WHERE project_id = ? AND status IN ('QUEUED','PROCESSING')", &stuck},
		{"review_pending", "SELECT COUNT(*) FROM tasks WHERE project_id = ? AND status = 'REVIEW_PENDING'", &reviewPending},
		{"unknown_result", "SELECT COUNT(*) FROM tasks WHERE project_id = ? AND result NOT IN ('PASS','REJECT','REVIEW')", &unknownResult},
		{"failed_routes", "SELECT COUNT(*) FROM tasks WHERE project_id = ? AND LOCATE('\"ok\":false', result_json) > 0", &failedRoutes},
		{"missing_callback", "SELECT COUNT(*) FROM tasks t JOIN audits a ON a.id=t.audit_id WHERE t.project_id = ? AND a.callback_enabled=1 AND t.status='SUCCESS' AND (JSON_EXTRACT(t.result_json,'$.callback.ok') IS NULL OR JSON_EXTRACT(t.result_json,'$.callback.ok') = false)", &missingCallback},
		{"failed_scripts", "SELECT COUNT(*) FROM script_executions WHERE project_id = ? AND status <> 'SUCCESS'", &failedScripts},
		{"missing_success_event", "SELECT COUNT(*) FROM tasks t WHERE t.project_id = ? AND t.status='SUCCESS' AND NOT EXISTS (SELECT 1 FROM task_events e WHERE e.task_id=t.id AND e.to_status='SUCCESS')", &missingSuccessEvent},
		{"duplicate_biz_keys", "SELECT COUNT(*) FROM (SELECT biz_key FROM tasks WHERE project_id = ? AND biz_key <> '' GROUP BY biz_key HAVING COUNT(*) > 1) x", &duplicateBizKeys},
		{"failed_task_logs", "SELECT COUNT(*) FROM audit_logs WHERE project_id = ? AND result='FAILED' AND action LIKE 'TASK_%'", &failedAuditLogs},
	}
	for _, item := range queries {
		if err := r.db.QueryRowContext(ctx, item.query, r.state.ProjectID).Scan(item.dest); err != nil {
			failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d %s query: %v", first, round, item.name, err))
		}
	}
	if taskCount != expected {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d task count=%d want %d", first, round, taskCount, expected))
	}
	if stuck != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d stuck tasks=%d", first, round, stuck))
	}
	if reviewPending != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d unreviewed tasks=%d", first, round, reviewPending))
	}
	if unknownResult != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d unknown results=%d", first, round, unknownResult))
	}
	if failedRoutes != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d failed result routes=%d", first, round, failedRoutes))
	}
	if missingCallback != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d missing/failed callbacks=%d", first, round, missingCallback))
	}
	if failedScripts != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d failed scripts=%d", first, round, failedScripts))
	}
	if missingSuccessEvent != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d missing success events=%d", first, round, missingSuccessEvent))
	}
	if duplicateBizKeys != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d duplicate biz keys=%d", first, round, duplicateBizKeys))
	}
	if failedAuditLogs != 0 {
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d failed task logs=%d", first, round, failedAuditLogs))
	}
	iter := r.actionRedis.Scan(ctx, 0, fmt.Sprintf("qa100:%s:*", r.state.RunID), 100).Iterator()
	leftovers := 0
	for iter.Next(ctx) {
		leftovers++
	}
	if leftovers != 0 {
		warnings = append(warnings, fmt.Sprintf("PAIR R%03d-R%03d redis leftovers=%d", first, round, leftovers))
	}
	status := "OK"
	if len(failures) > 0 {
		status = "FAIL"
	}
	r.latencyMu.Lock()
	submit := append([]time.Duration(nil), r.submitLatencies...)
	tasks := append([]time.Duration(nil), r.taskLatencies...)
	r.latencyMu.Unlock()
	sort.Slice(submit, func(i, j int) bool { return submit[i] < submit[j] })
	sort.Slice(tasks, func(i, j int) bool { return tasks[i] < tasks[j] })
	p95Submit, p95Task := percentile(submit, 0.95), percentile(tasks, 0.95)
	fmt.Printf("PAIR R%03d-R%03d %s tasks=%d expect=%d stuck=%d pending=%d failed_routes=%d failed_scripts=%d p95_submit=%s p95_task=%s\n",
		first, round, status, taskCount, expected, stuck, reviewPending, failedRoutes, failedScripts, p95Submit, p95Task)
	for _, failure := range failures {
		fmt.Printf("  ! %s\n", failure)
	}
	return failures, warnings
}

func (r *runner) runDedupeRace(ctx context.Context, round int, cfg *configSpec) ([]string, []string, int) {
	size := r.raceSize
	if size < 2 {
		size = 8
	}
	bizKey := fmt.Sprintf("qa100:%s:r%03d:a:race:%d", r.state.RunID, round, time.Now().UnixNano())
	text := fmt.Sprintf("QA_PASS concurrent dedupe race round=%d", round)
	ext := map[string]any{"race": true, "round": round}
	type result struct {
		task       *task
		duplicated bool
		err        error
	}
	results := make([]result, size)
	var wg sync.WaitGroup
	for i := 0; i < size; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			t, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, cfg.AuditID, bizKey, text, ext, nil)
			results[i] = result{task: t, duplicated: duplicated, err: err}
		}()
	}
	wg.Wait()
	var failures, warnings []string
	var created []*task
	var anyTask *task
	for i, result := range results {
		if result.err != nil {
			failures = append(failures, fmt.Sprintf("R%03d RACE submit %d: %v", round, i+1, result.err))
			continue
		}
		if result.task == nil {
			failures = append(failures, fmt.Sprintf("R%03d RACE submit %d returned nil task", round, i+1))
			continue
		}
		if anyTask == nil {
			anyTask = result.task
		}
		if !result.duplicated {
			created = append(created, result.task)
		} else if anyTask != nil && result.task.ID != anyTask.ID {
			failures = append(failures, fmt.Sprintf("R%03d RACE duplicate references task %d, first %d", round, result.task.ID, anyTask.ID))
		}
	}
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE biz_key = ?", bizKey).Scan(&count); err != nil {
		failures = append(failures, fmt.Sprintf("R%03d RACE count: %v", round, err))
	}
	if count > 0 {
		r.latencyMu.Lock()
		r.expectedTasks += count
		r.latencyMu.Unlock()
	}
	if count != 1 {
		failures = append(failures, fmt.Sprintf("R%03d RACE stored tasks=%d want 1", round, count))
	}
	if len(created) > 1 {
		failures = append(failures, fmt.Sprintf("R%03d RACE created=%d want 1", round, len(created)))
	}
	if len(created) == 0 {
		failures = append(failures, fmt.Sprintf("R%03d RACE no request returned created=false", round))
	}
	if anyTask == nil {
		return failures, warnings, 0
	}
	c := &caseSpec{
		Round: round, ConfigKey: "a", AuditID: cfg.AuditID, Secret: cfg.CallbackSecret,
		Kind: "RACE", Expected: "PASS", Marker: "QA_PASS",
		Task: anyTask, TaskID: anyTask.ID, TaskNo: anyTask.TaskNo,
		SubmittedAt: time.Now(),
	}
	r.waitCase(ctx, c)
	r.verifyCase(ctx, c)
	failures = append(failures, prefixFailures(round, "a/RACE", anyTask.TaskNo, c.Failures)...)
	warnings = append(warnings, prefixWarnings(round, "a/RACE", anyTask.TaskNo, c.Warnings)...)
	return failures, warnings, 1
}
