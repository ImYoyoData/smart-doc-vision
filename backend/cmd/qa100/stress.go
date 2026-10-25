package main

import (
	"context"
	"fmt"
	"sort"
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
		{"missing_callback", "SELECT COUNT(*) FROM tasks WHERE project_id = ? AND status='SUCCESS' AND (JSON_EXTRACT(result_json,'$.callback.ok') IS NULL OR JSON_EXTRACT(result_json,'$.callback.ok') = false)", &missingCallback},
		{"failed_scripts", "SELECT COUNT(*) FROM script_executions WHERE project_id = ? AND status <> 'SUCCESS'", &failedScripts},
		{"missing_success_event", "SELECT COUNT(*) FROM tasks t WHERE t.project_id = ? AND t.status='SUCCESS' AND NOT EXISTS (SELECT 1 FROM task_events e WHERE e.task_id=t.id AND e.to_status='SUCCESS')", &missingSuccessEvent},
		{"duplicate_biz_keys", "SELECT COUNT(*) FROM (SELECT biz_key FROM tasks WHERE project_id = ? AND biz_key <> '' GROUP BY biz_key HAVING COUNT(*) > 1) x", &duplicateBizKeys},
		{"failed_audit_logs", "SELECT COUNT(*) FROM audit_logs WHERE project_id = ? AND result='FAILED'", &failedAuditLogs},
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
		failures = append(failures, fmt.Sprintf("PAIR R%03d-R%03d failed audit logs=%d", first, round, failedAuditLogs))
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
