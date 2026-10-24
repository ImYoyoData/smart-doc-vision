package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type configSpec struct {
	Key            string
	Code           string
	Name           string
	Mode           string
	Strictness     string
	CallbackKind   string
	CallbackSecret string
	IncludeContent bool
	DedupeWindow   int
	AuditID        uint64
	Cases          []*caseSpec
}

type caseSpec struct {
	Round       int
	ConfigKey   string
	AuditID     uint64
	Secret      string
	Kind        string
	Expected    string
	Marker      string
	Text        string
	BizKey      string
	TaskID      uint64
	TaskNo      string
	Task        *task
	Events      []map[string]any
	FinalResult string
	Conclusion  string
	Failures    []string
	Warnings    []string
}

func (c *caseSpec) fail(format string, args ...any) {
	c.Failures = append(c.Failures, fmt.Sprintf(format, args...))
}

func (c *caseSpec) warn(format string, args ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, args...))
}

type roundReport struct {
	Round      int          `json:"round"`
	DurationMS int64        `json:"duration_ms"`
	Cases      int          `json:"cases"`
	Reviews    int          `json:"reviews"`
	Failures   []string     `json:"failures,omitempty"`
	Warnings   []string     `json:"warnings,omitempty"`
	Tasks      []caseReport `json:"tasks,omitempty"`
}

type caseReport struct {
	Config     string   `json:"config"`
	Kind       string   `json:"kind"`
	TaskNo     string   `json:"task_no,omitempty"`
	Expected   string   `json:"expected"`
	Actual     string   `json:"actual,omitempty"`
	Status     string   `json:"status,omitempty"`
	DurationMS int64    `json:"duration_ms,omitempty"`
	Failures   []string `json:"failures,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

type runner struct {
	env         *testEnv
	client      *apiClient
	state       state
	mock        *mockModel
	actions     *actionServer
	appRedis    *redis.Client
	actionRedis *redis.Client
	db          *sql.DB
	roundStart  int
}

func setupTestProject(ctx context.Context, c *apiClient, te *testEnv) (state, error) {
	runID := time.Now().Format("060102150405")
	projectCode := "qa100-" + runID
	p, err := c.createProject(ctx, "QA100 自动化测试 "+runID, projectCode)
	if err != nil {
		return state{}, fmt.Errorf("create project: %w", err)
	}
	providerBase := "http://" + te.mockAddr + "/v1"
	prov, err := c.createProvider(ctx, p.ID, "QA100 Mock "+runID, providerBase, "qa-mock")
	if err != nil {
		return state{}, fmt.Errorf("create provider: %w", err)
	}
	if err := c.testProvider(ctx, prov.ID); err != nil {
		return state{}, fmt.Errorf("test provider: %w", err)
	}
	conn, err := c.createRedisConnection(ctx, p.ID, "QA100 Redis "+runID, te.redisAddr)
	if err != nil {
		return state{}, fmt.Errorf("create redis connection: %w", err)
	}
	if err := c.testRedisConnection(ctx, conn.ID); err != nil {
		return state{}, fmt.Errorf("test redis connection: %w", err)
	}
	return state{
		RunID: runID, ProjectID: p.ID, ProjectCode: p.Code,
		ProviderID: prov.ID, RedisConnID: conn.ID, CreatedAt: time.Now(),
	}, nil
}

func verifyTestProject(ctx context.Context, c *apiClient, st state) error {
	p, err := c.getProject(ctx, st.ProjectID)
	if err != nil {
		return err
	}
	if p.Code != st.ProjectCode {
		return fmt.Errorf("project code mismatch: got %s want %s", p.Code, st.ProjectCode)
	}
	return nil
}

func openTestDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(6)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (r *runner) runRounds(ctx context.Context, start, end int) error {
	reportFile, err := os.OpenFile(r.env.reportPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer reportFile.Close()
	var totalCases, totalReviews, totalFailures, totalWarnings int
	var totalDuration time.Duration
	for round := start; round <= end; round++ {
		report := r.runRound(ctx, round)
		totalDuration += time.Duration(report.DurationMS) * time.Millisecond
		totalCases += report.Cases
		totalReviews += report.Reviews
		totalFailures += len(report.Failures)
		totalWarnings += len(report.Warnings)
		raw, _ := json.Marshal(report)
		_, _ = reportFile.Write(append(raw, '\n'))
		status := "OK"
		if len(report.Failures) > 0 {
			status = "FAIL"
		}
		fmt.Printf("R%03d %s cases=%d reviews=%d %.1fs failures=%d warnings=%d\n",
			round, status, report.Cases, report.Reviews, float64(report.DurationMS)/1000, len(report.Failures), len(report.Warnings))
		for _, failure := range report.Failures {
			fmt.Printf("  ! %s\n", failure)
		}
	}
	rounds := end - start + 1
	if rounds > 0 {
		fmt.Printf("BATCH %d-%d rounds=%d cases=%d reviews=%d failures=%d warnings=%d avg=%.1fs\n",
			start, end, rounds, totalCases, totalReviews, totalFailures, totalWarnings,
			totalDuration.Seconds()/float64(rounds))
	}
	return nil
}

func (r *runner) runRound(ctx context.Context, round int) roundReport {
	started := time.Now()
	report := roundReport{Round: round}
	configs := r.makeConfigs(round)
	var active []*configSpec
	for _, cfg := range configs {
		if err := r.createAudit(ctx, round, cfg); err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("R%03d config %s create: %v", round, cfg.Key, err))
			continue
		}
		active = append(active, cfg)
	}
	var cases []*caseSpec
	for _, cfg := range active {
		cfg.Cases = r.makeCases(round, cfg)
		cases = append(cases, cfg.Cases...)
	}
	r.submitCases(ctx, cases)
	r.waitCases(ctx, cases)
	r.reviewCases(ctx, round, cases)
	r.verifyCases(ctx, cases)

	for _, c := range cases {
		report.Cases++
		if c.Conclusion != "" {
			report.Reviews++
		}
		for _, failure := range c.Failures {
			report.Failures = append(report.Failures, fmt.Sprintf("R%03d %s/%s %s: %s", round, c.ConfigKey, c.Kind, c.TaskNo, failure))
		}
		for _, warning := range c.Warnings {
			report.Warnings = append(report.Warnings, fmt.Sprintf("R%03d %s/%s %s: %s", round, c.ConfigKey, c.Kind, c.TaskNo, warning))
		}
		report.Tasks = append(report.Tasks, caseReport{
			Config: c.ConfigKey, Kind: c.Kind, TaskNo: c.TaskNo,
			Expected: c.Expected, Actual: c.FinalResult,
			Status: taskStatus(c.Task), DurationMS: taskDuration(c.Task),
			Failures: c.Failures, Warnings: c.Warnings,
		})
	}

	if cfgA := findConfig(active, "a"); cfgA != nil {
		extraFailures, extraWarnings, extraCases, extraReviews := r.runDuplicateTest(ctx, round, cfgA)
		report.Failures = append(report.Failures, extraFailures...)
		report.Warnings = append(report.Warnings, extraWarnings...)
		report.Cases += extraCases
		report.Reviews += extraReviews
		if round%10 == 0 {
			failures, warnings := r.runValidationTests(ctx, round, cfgA)
			report.Failures = append(report.Failures, failures...)
			report.Warnings = append(report.Warnings, warnings...)
		}
	}
	if _, err := r.client.reviewSummary(ctx, r.state.ProjectID); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("R%03d review summary: %v", round, err))
	}
	if round%20 == 0 {
		if cfgC := findConfig(active, "c"); cfgC != nil {
			edgeCases := r.makeEdgeCases(round, cfgC)
			r.submitCases(ctx, edgeCases)
			r.waitCases(ctx, edgeCases)
			r.reviewCases(ctx, round, edgeCases)
			r.verifyCases(ctx, edgeCases)
			for _, c := range edgeCases {
				report.Cases++
				if c.Conclusion != "" {
					report.Reviews++
				}
				for _, failure := range c.Failures {
					report.Failures = append(report.Failures, fmt.Sprintf("R%03d %s/%s %s: %s", round, c.ConfigKey, c.Kind, c.TaskNo, failure))
				}
				for _, warning := range c.Warnings {
					report.Warnings = append(report.Warnings, fmt.Sprintf("R%03d %s/%s %s: %s", round, c.ConfigKey, c.Kind, c.TaskNo, warning))
				}
			}
		}
	}

	r.cleanupRoundRedis(ctx, round)
	report.DurationMS = time.Since(started).Milliseconds()
	return report
}

func findConfig(configs []*configSpec, key string) *configSpec {
	for _, cfg := range configs {
		if cfg.Key == key {
			return cfg
		}
	}
	return nil
}

func (r *runner) makeConfigs(round int) []*configSpec {
	prefix := fmt.Sprintf("qa100_%s_r%03d", r.state.RunID, round)
	base := func(key string) *configSpec {
		return &configSpec{
			Key:            key,
			Code:           prefix + "_" + key,
			Name:           fmt.Sprintf("QA100 R%03d %s", round, strings.ToUpper(key)),
			CallbackSecret: fmt.Sprintf("qa100-secret-%s-%s-%03d", r.state.RunID, key, round),
		}
	}
	a := base("a")
	a.Mode = "REVIEW"
	a.Strictness = "standard"
	a.CallbackKind = "http"
	a.IncludeContent = true
	a.DedupeWindow = 120
	a.Name += " Review Standard"
	b := base("b")
	b.Mode = "REVIEW"
	b.Strictness = "strict"
	b.CallbackKind = "ws"
	b.IncludeContent = true
	b.Name += " Review Strict"
	c := base("c")
	c.Mode = "CLASSIFY"
	c.CallbackKind = "http"
	c.IncludeContent = false
	c.Name += " Classify"
	return []*configSpec{a, b, c}
}

func (r *runner) createAudit(ctx context.Context, round int, cfg *configSpec) error {
	callbackURL := "http://" + r.env.actionAddr + "/callback/http"
	if cfg.CallbackKind == "ws" {
		callbackURL = "ws://" + r.env.actionAddr + "/ws-callback"
	}
	payload := map[string]any{
		"project_id": r.state.ProjectID, "name": cfg.Name, "code": cfg.Code,
		"description": "QA100 automated audit " + cfg.Key,
		"media_types": []string{"text"}, "decision_mode": cfg.Mode,
		"provider_id":    r.state.ProviderID,
		"review_prompt":  "QA100 automated review. Return the requested strict JSON verdict.",
		"default_result": "REVIEW", "on_model_fail": "REVIEW",
		"dedupe_window_sec": cfg.DedupeWindow, "status": "ACTIVE",
		"callback_enabled": true, "callback_url": callbackURL,
		"callback_secret":          cfg.CallbackSecret,
		"callback_results":         []string{"PASS", "REJECT"},
		"callback_headers":         map[string]string{"X-QA-Round": fmt.Sprint(round), "X-QA-Config": cfg.Key},
		"callback_include_content": cfg.IncludeContent,
		"routes":                   r.buildRoutes(round, cfg),
	}
	if cfg.Strictness != "" {
		payload["strictness"] = cfg.Strictness
	}
	if cfg.Mode == "CLASSIFY" {
		payload["categories"] = []map[string]any{
			{"code": "safe", "name": "安全", "description": "低风险内容", "result": "PASS"},
			{"code": "risk", "name": "风险", "description": "高风险内容", "result": "REJECT"},
			{"code": "manual", "name": "待定", "description": "需要人工确认", "result": "REVIEW"},
		}
	}
	audit, err := r.client.createAudit(ctx, payload)
	if err != nil {
		return err
	}
	cfg.AuditID = audit.ID
	return nil
}

func (r *runner) buildRoutes(round int, cfg *configSpec) map[string]any {
	return map[string]any{
		"PASS": map[string]any{
			"exec_mode": "sequential", "on_error": "continue",
			"actions": r.actionsFor(round, cfg, "PASS"),
		},
		"REJECT": map[string]any{
			"exec_mode": "sequential", "on_error": "continue",
			"actions": r.actionsFor(round, cfg, "REJECT"),
		},
		"REVIEW": map[string]any{
			"exec_mode": "sequential", "on_error": "to_review",
			"actions": []any{},
		},
	}
}

func (r *runner) actionsFor(round int, cfg *configSpec, result string) []any {
	actionBase := "http://" + r.env.actionAddr
	wsBase := "ws://" + r.env.actionAddr
	payload := fmt.Sprintf(`{"kind":"%s","task":"${task_no}","result":"${result}","audit":"${audit_code}","biz":"${biz_key}"}`, strings.ToLower(result))
	redisTarget := fmt.Sprintf("qa100:%s:r%03d:%s:pass:${task_no}", r.state.RunID, round, cfg.Key)
	redisCommand := "set"
	if result == "REJECT" {
		redisTarget = fmt.Sprintf("qa100:%s:r%03d:%s:reject", r.state.RunID, round, cfg.Key)
		redisCommand = "xadd"
	}
	jsScript := fmt.Sprintf(`function main(ctx) {
const payload = ctx.task.task_no + '|' + ctx.result;
const body = '{"kind":"js","task":"' + ctx.task.task_no + '","result":"' + ctx.result + '","audit":"' + ctx.audit.code + '","hash":"' + ctx.crypto.sha256(payload) + '"}';
const resp = ctx.http.request({url: '%s/action/js', method: 'POST', headers: {'Content-Type': 'application/json'}, body: body});
return {ok: resp.ok, status: resp.status, hash: ctx.crypto.sha256(payload)};
}`, actionBase)
	return []any{
		map[string]any{
			"type": "http", "method": "POST", "url": actionBase + "/action/http",
			"headers": map[string]string{"X-QA": "http"}, "body": payload,
			"timeout_ms": 5000, "remark": "qa-http",
		},
		map[string]any{
			"type": "websocket", "url": wsBase + "/ws-action",
			"payload": payload, "timeout_ms": 5000, "remark": "qa-ws",
		},
		map[string]any{
			"type": "redis", "connection_id": r.state.RedisConnID,
			"command": redisCommand, "target": redisTarget, "payload": payload,
			"db": 14, "remark": "qa-redis",
		},
		map[string]any{
			"type": "js", "script": jsScript, "timeout_ms": 5000,
			"memory_mb": 32, "remark": "qa-js",
		},
	}
}

func (r *runner) makeCases(round int, cfg *configSpec) []*caseSpec {
	kinds := []string{"PASS", "REJECT", "REVIEW"}
	out := make([]*caseSpec, 0, len(kinds))
	for _, kind := range kinds {
		marker := "QA_" + kind
		if cfg.Mode == "CLASSIFY" {
			marker = "QA_CLASSIFY_" + kind
		}
		nonce := time.Now().UnixNano()
		c := &caseSpec{
			Round: round, ConfigKey: cfg.Key, AuditID: cfg.AuditID, Secret: cfg.CallbackSecret,
			Kind: kind, Expected: kind, Marker: marker,
			Text:   fmt.Sprintf("%s round=%d config=%s case=%s nonce=%d\n合同编号 QA100-%d-%s，金额 123 万元，请审核。", marker, round, cfg.Key, kind, nonce, round, cfg.Key),
			BizKey: fmt.Sprintf("qa100:%s:r%03d:%s:%s:%d", r.state.RunID, round, cfg.Key, strings.ToLower(kind), nonce),
		}
		out = append(out, c)
	}
	cfg.Cases = out
	return out
}

func (r *runner) makeEdgeCases(round int, cfg *configSpec) []*caseSpec {
	markers := []string{"QA_LOW_CONF", "QA_UNKNOWN"}
	out := make([]*caseSpec, 0, len(markers))
	for index, marker := range markers {
		nonce := time.Now().UnixNano() + int64(index)
		out = append(out, &caseSpec{
			Round: round, ConfigKey: cfg.Key, AuditID: cfg.AuditID, Secret: cfg.CallbackSecret,
			Kind: "EDGE_" + marker, Expected: "REVIEW", Marker: marker,
			Text:   fmt.Sprintf("%s round=%d config=%s edge=%d", marker, round, cfg.Key, index),
			BizKey: fmt.Sprintf("qa100:%s:r%03d:%s:edge%d:%d", r.state.RunID, round, cfg.Key, index, nonce),
		})
	}
	return out
}

func (r *runner) submitCases(ctx context.Context, cases []*caseSpec) {
	var wg sync.WaitGroup
	for _, c := range cases {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			t, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, c.AuditID, c.BizKey, c.Text,
				map[string]any{"round": c.Round, "config": c.ConfigKey, "kind": c.Kind}, nil)
			if err != nil {
				c.fail("submit: %v", err)
				return
			}
			if duplicated {
				c.fail("unexpected duplicate on first submit")
			}
			c.Task = t
			c.TaskID = t.ID
			c.TaskNo = t.TaskNo
		}()
	}
	wg.Wait()
}

func (r *runner) waitCases(ctx context.Context, cases []*caseSpec) {
	var wg sync.WaitGroup
	for _, c := range cases {
		c := c
		if c.TaskID == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.waitCase(ctx, c)
		}()
	}
	wg.Wait()
}

func (r *runner) waitCase(ctx context.Context, c *caseSpec) {
	deadline := time.Now().Add(45 * time.Second)
	transient := 0
	for {
		t, events, err := r.client.getTask(ctx, c.TaskID)
		if err != nil {
			transient++
			if transient > 20 {
				c.fail("get task: %v", err)
				return
			}
			time.Sleep(150 * time.Millisecond)
			continue
		}
		transient = 0
		c.Task = t
		c.Events = events
		switch t.Status {
		case "SUCCESS", "FAILED", "REVIEW_PENDING", "STOPPED":
			if t.Status == "FAILED" {
				c.fail("task failed: %s", t.ErrorMessage)
			}
			if c.Expected == "REVIEW" && t.Status == "REVIEW_PENDING" {
				r.checkPreReviewState(ctx, c)
			}
			if c.Expected != "REVIEW" && t.Status == "REVIEW_PENDING" {
				c.fail("unexpected REVIEW_PENDING for expected %s", c.Expected)
			}
			if c.Expected == "REVIEW" && t.Status == "SUCCESS" {
				c.fail("expected REVIEW_PENDING, got SUCCESS result=%s", t.Result)
			}
			c.FinalResult = t.Result
			return
		}
		if time.Now().After(deadline) {
			c.fail("timeout waiting task status=%s", t.Status)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (r *runner) checkPreReviewState(ctx context.Context, c *caseSpec) {
	if routes := routeEntries(c.Task.ResultDetail); len(routes) != 0 {
		c.fail("REVIEW task executed %d result actions before human review", len(routes))
	}
	for _, kind := range []string{"http-action", "ws-action", "js-action", "http-callback", "ws-callback"} {
		if items := r.actions.rec.forTask(kind, c.TaskNo); len(items) > 0 {
			c.fail("REVIEW task emitted %s before human review", kind)
		}
	}
}

func (r *runner) reviewCases(ctx context.Context, round int, cases []*caseSpec) {
	var wg sync.WaitGroup
	for index, c := range cases {
		c := c
		if c.Task == nil || c.Task.Status != "REVIEW_PENDING" {
			continue
		}
		conclusion := "PASS"
		if (round+index)%2 == 1 {
			conclusion = "REJECT"
		}
		c.Conclusion = conclusion
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.client.reviewTask(ctx, c.TaskID, conclusion, "QA100_"+conclusion, "QA100 automated human review"); err != nil {
				c.fail("review: %v", err)
				return
			}
			deadline := time.Now().Add(30 * time.Second)
			for {
				t, events, err := r.client.getTask(ctx, c.TaskID)
				if err == nil {
					c.Task = t
					c.Events = events
					if t.Status == "SUCCESS" || t.Status == "FAILED" {
						c.FinalResult = t.Result
						if t.Status == "FAILED" {
							c.fail("task failed after review: %s", t.ErrorMessage)
						}
						if t.Result != conclusion {
							c.fail("review result=%s want %s", t.Result, conclusion)
						}
						return
					}
				}
				if time.Now().After(deadline) {
					c.fail("timeout waiting reviewed task")
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
}

func (r *runner) verifyCases(ctx context.Context, cases []*caseSpec) {
	for _, c := range cases {
		if c.Task == nil {
			continue
		}
		r.verifyCase(ctx, c)
	}
}

func (r *runner) verifyCase(ctx context.Context, c *caseSpec) {
	final := c.FinalResult
	if final == "" {
		final = c.Expected
	}
	if c.Task.Status != "SUCCESS" {
		c.fail("final status=%s want SUCCESS", c.Task.Status)
	}
	if c.Task.Result != final {
		c.fail("final result=%s want %s", c.Task.Result, final)
	}
	if c.Task.ErrorMessage != "" {
		c.warn("task error_message: %s", c.Task.ErrorMessage)
	}
	r.verifyRoutes(c)
	r.verifyActions(c, final)
	r.verifyRedis(ctx, c, final)
	r.verifyCallback(ctx, c, final)
	r.verifyScriptExecution(ctx, c)
	r.verifyEvents(c)
	if c.Task.DurationMS > 5000 {
		c.warn("slow task duration=%dms", c.Task.DurationMS)
	}
}

func (r *runner) verifyRoutes(c *caseSpec) {
	routes := routeEntries(c.Task.ResultDetail)
	if len(routes) != 4 {
		c.fail("result routes count=%d want 4", len(routes))
	}
	types := map[string]bool{}
	for _, route := range routes {
		kind := strings.ToLower(fmt.Sprint(route["type"]))
		types[kind] = true
		if ok, exists := route["ok"]; !exists || fmt.Sprint(ok) != "true" {
			c.fail("route %s failed: %v", kind, route["error"])
		}
		if duration := uint64FromAny(route["duration_ms"]); duration > 3000 {
			c.warn("slow result action %s duration=%dms", kind, duration)
		}
	}
	for _, kind := range []string{"http", "websocket", "redis", "js"} {
		if !types[kind] {
			c.fail("missing result route type %s", kind)
		}
	}
}

func (r *runner) verifyActions(c *caseSpec, final string) {
	for _, kind := range []string{"http-action", "ws-action", "js-action"} {
		items, err := r.actions.rec.wait(kind, c.TaskNo, 1, 15*time.Second)
		if err != nil {
			c.fail("%s callback: %v", kind, err)
			continue
		}
		if len(items) != 1 {
			c.fail("%s count=%d want 1", kind, len(items))
		}
		item := items[0]
		taskNo, result, auditCode := fieldsFromJSON([]byte(item.Body))
		if taskNo != c.TaskNo {
			c.fail("%s task field=%q want %q", kind, taskNo, c.TaskNo)
		}
		if result != final {
			c.fail("%s result field=%q want %q", kind, result, final)
		}
		if auditCode == "" {
			c.fail("%s missing audit code", kind)
		}
	}
}

func (r *runner) verifyRedis(ctx context.Context, c *caseSpec, final string) {
	if final == "PASS" {
		key := fmt.Sprintf("qa100:%s:r%03d:%s:pass:%s", r.state.RunID, c.Round, c.ConfigKey, c.TaskNo)
		raw, err := r.actionRedis.Get(ctx, key).Result()
		if err != nil {
			c.fail("redis SET key missing: %v", err)
			return
		}
		if !strings.Contains(raw, c.TaskNo) || !strings.Contains(raw, "PASS") {
			c.fail("redis SET payload mismatch: %s", raw)
		}
		return
	}
	if final == "REJECT" {
		stream := fmt.Sprintf("qa100:%s:r%03d:%s:reject", r.state.RunID, c.Round, c.ConfigKey)
		items, err := r.actionRedis.XRange(ctx, stream, "-", "+").Result()
		if err != nil {
			c.fail("redis XADD stream missing: %v", err)
			return
		}
		hits := 0
		for _, item := range items {
			data := fmt.Sprint(item.Values["data"])
			if strings.Contains(data, c.TaskNo) {
				hits++
				if !strings.Contains(data, "REJECT") {
					c.fail("redis XADD payload missing result: %s", data)
				}
			}
		}
		if hits != 1 {
			c.fail("redis XADD hits=%d want 1", hits)
		}
	}
}

func (r *runner) verifyCallback(ctx context.Context, c *caseSpec, final string) {
	if final != "PASS" && final != "REJECT" {
		return
	}
	kind := "http-callback"
	if c.ConfigKey == "b" {
		kind = "ws-callback"
	}
	items, err := r.actions.rec.wait(kind, c.TaskNo, 1, 20*time.Second)
	if err != nil {
		c.fail("audit callback: %v", err)
		return
	}
	if len(items) != 1 {
		c.fail("audit callback count=%d want 1", len(items))
	}
	item := items[0]
	if item.Result != final {
		c.fail("callback record result=%s want %s", item.Result, final)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(item.Body), &body); err != nil {
		c.fail("callback body invalid JSON: %v", err)
		return
	}
	if fmt.Sprint(body["event"]) != "audit.completed" {
		c.fail("callback event=%v", body["event"])
	}
	if fmt.Sprint(body["task_no"]) != c.TaskNo {
		c.fail("callback task_no=%v", body["task_no"])
	}
	if fmt.Sprint(body["result"]) != final {
		c.fail("callback body result=%v want %s", body["result"], final)
	}
	if fmt.Sprint(body["audit_code"]) == "" || fmt.Sprint(body["audit_code"]) == "<nil>" {
		c.fail("callback missing audit_code")
	}
	secret := c.Secret
	signature := item.Headers.Get("X-SmartDoc-Signature")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(item.Body))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if signature != want {
		c.fail("callback signature mismatch")
	}
	if item.Headers.Get("X-SmartDoc-Task") != c.TaskNo {
		c.fail("callback header X-SmartDoc-Task=%q", item.Headers.Get("X-SmartDoc-Task"))
	}
	if item.Headers.Get("X-SmartDoc-Result") != final {
		c.fail("callback header X-SmartDoc-Result=%q", item.Headers.Get("X-SmartDoc-Result"))
	}
	if item.Headers.Get("X-Idempotency-Key") != c.TaskNo {
		c.fail("callback idempotency header=%q", item.Headers.Get("X-Idempotency-Key"))
	}
	if item.Headers.Get("X-QA-Round") == "" {
		c.fail("callback missing custom X-QA-Round header")
	}
	media, _ := body["media"].([]any)
	include := c.ConfigKey != "c"
	for _, entry := range media {
		m, _ := entry.(map[string]any)
		content, hasContent := m["content"]
		if include && (!hasContent || fmt.Sprint(content) == "") {
			c.fail("callback include_content=true but media content missing")
		}
		if !include && hasContent {
			c.fail("callback include_content=false but media content present")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		t, _, err := r.client.getTask(ctx, c.TaskID)
		if err == nil {
			c.Task = t
			if cb, ok := t.ResultDetail["callback"].(map[string]any); ok {
				if fmt.Sprint(cb["ok"]) == "true" {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			c.warn("result_detail.callback not marked ok before timeout")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (r *runner) verifyScriptExecution(ctx context.Context, c *caseSpec) {
	executions, err := r.client.listScriptExecutions(ctx, c.AuditID, c.TaskNo)
	if err != nil {
		c.fail("list script executions: %v", err)
		return
	}
	var successes int
	for _, execution := range executions {
		if execution.TaskNo != c.TaskNo {
			continue
		}
		if execution.Status == "SUCCESS" {
			successes++
		} else {
			c.fail("script execution status=%s error=%s", execution.Status, execution.Error)
		}
	}
	if successes != 1 {
		c.fail("successful script executions=%d want 1", successes)
	}
	var resultJSON string
	if err := r.db.QueryRowContext(ctx, "SELECT result_json FROM script_executions WHERE task_id = ? AND status = 'SUCCESS' ORDER BY id DESC LIMIT 1", c.TaskID).Scan(&resultJSON); err != nil {
		c.fail("query script result: %v", err)
		return
	}
	if !strings.Contains(resultJSON, `"ok":true`) || !strings.Contains(resultJSON, `"hash"`) {
		c.fail("script result payload unexpected: %s", resultJSON)
	}
}

func (r *runner) verifyEvents(c *caseSpec) {
	if len(c.Events) == 0 {
		c.warn("task events missing")
		return
	}
	hasSuccess := false
	for _, event := range c.Events {
		if fmt.Sprint(event["to_status"]) == "SUCCESS" {
			hasSuccess = true
			break
		}
	}
	if !hasSuccess {
		time.Sleep(120 * time.Millisecond)
		if _, events, err := r.client.getTask(context.Background(), c.TaskID); err == nil {
			c.Events = events
			for _, event := range c.Events {
				if fmt.Sprint(event["to_status"]) == "SUCCESS" {
					hasSuccess = true
					break
				}
			}
		}
	}
	if !hasSuccess {
		c.warn("task event timeline missing SUCCESS")
	}
	if c.Conclusion != "" {
		hasReview := false
		for _, event := range c.Events {
			message := stringValue(event["message"])
			if strings.Contains(message, "人工复核") {
				hasReview = true
				break
			}
		}
		if !hasReview {
			c.warn("task event timeline missing human review event")
		}
	}
}

func (r *runner) runDuplicateTest(ctx context.Context, round int, cfg *configSpec) ([]string, []string, int, int) {
	var failures, warnings []string
	nonce := time.Now().UnixNano()
	bizKey := fmt.Sprintf("qa100:%s:r%03d:a:dup:%d", r.state.RunID, round, nonce)
	text := fmt.Sprintf("QA_PASS duplicate round=%d nonce=%d", round, nonce)
	ext := map[string]any{"round": round, "config": "a", "kind": "DUPLICATE"}
	first, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, cfg.AuditID, bizKey, text, ext, nil)
	if err != nil {
		return []string{fmt.Sprintf("R%03d duplicate first submit: %v", round, err)}, nil, 0, 0
	}
	if duplicated {
		failures = append(failures, fmt.Sprintf("R%03d duplicate first submit marked duplicated", round))
	}
	c := &caseSpec{
		Round: round, ConfigKey: "a", AuditID: cfg.AuditID, Secret: cfg.CallbackSecret,
		Kind: "DUPLICATE", Expected: "PASS", Marker: "QA_PASS",
		Task: first, TaskID: first.ID, TaskNo: first.TaskNo, FinalResult: "PASS",
	}
	r.waitCase(ctx, c)
	r.verifyCase(ctx, c)
	failures = append(failures, prefixFailures(round, "a/DUPLICATE", first.TaskNo, c.Failures)...)
	warnings = append(warnings, prefixWarnings(round, "a/DUPLICATE", first.TaskNo, c.Warnings)...)
	second, duplicated, err := r.client.submitTask(ctx, r.state.ProjectID, cfg.AuditID, bizKey, text, ext, nil)
	if err != nil {
		failures = append(failures, fmt.Sprintf("R%03d duplicate second submit: %v", round, err))
		return failures, warnings, 1, 0
	}
	if !duplicated {
		failures = append(failures, fmt.Sprintf("R%03d duplicate second submit not marked duplicated", round))
	}
	if second.ID != first.ID {
		failures = append(failures, fmt.Sprintf("R%03d duplicate created new task %d vs %d", round, second.ID, first.ID))
	}
	time.Sleep(300 * time.Millisecond)
	latest := c.Task
	if latest == nil {
		latest = first
	}
	c2 := &caseSpec{Round: round, ConfigKey: "a", AuditID: cfg.AuditID, Secret: cfg.CallbackSecret, Kind: "DUPLICATE", Expected: "PASS", Task: latest, Events: c.Events, TaskID: first.ID, TaskNo: first.TaskNo, FinalResult: "PASS"}
	r.verifyCase(ctx, c2)
	failures = append(failures, prefixFailures(round, "a/DUPLICATE", first.TaskNo, c2.Failures)...)
	warnings = append(warnings, prefixWarnings(round, "a/DUPLICATE", first.TaskNo, c2.Warnings)...)
	var hash string
	if err := r.db.QueryRowContext(ctx, "SELECT dedupe_hash FROM tasks WHERE id = ?", first.ID).Scan(&hash); err != nil {
		failures = append(failures, fmt.Sprintf("R%03d duplicate query hash: %v", round, err))
	} else if hash == "" {
		failures = append(failures, fmt.Sprintf("R%03d duplicate task has empty dedupe hash", round))
	} else {
		key := fmt.Sprintf("sdv:%d:dedupe:%s", r.state.ProjectID, hash)
		value, err := r.appRedis.Get(ctx, key).Result()
		if err != nil || value != fmt.Sprint(first.ID) {
			failures = append(failures, fmt.Sprintf("R%03d duplicate redis cache missing/mismatch err=%v value=%q", round, err, value))
		}
	}
	return failures, warnings, 1, 0
}

func (r *runner) runValidationTests(ctx context.Context, round int, cfg *configSpec) ([]string, []string) {
	var failures, warnings []string
	nonce := time.Now().UnixNano()
	_, _, err := r.client.submitTask(ctx, r.state.ProjectID, cfg.AuditID,
		fmt.Sprintf("qa100:%s:r%03d:invalid-media:%d", r.state.RunID, round, nonce),
		"", map[string]any{"round": round, "kind": "INVALID_MEDIA"},
		[]map[string]any{{"type": "video", "content": "not-supported"}})
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		failures = append(failures, fmt.Sprintf("R%03d invalid media accepted or wrong error: %v", round, err))
	} else if apiErr.Status != 400 && apiErr.Status != 422 {
		failures = append(failures, fmt.Sprintf("R%03d invalid media status=%d code=%s", round, apiErr.Status, apiErr.Code))
	}
	if err := r.client.updateAuditStatus(ctx, cfg.AuditID, "PAUSED"); err != nil {
		failures = append(failures, fmt.Sprintf("R%03d pause audit: %v", round, err))
		return failures, warnings
	}
	_, _, pausedErr := r.client.submitTask(ctx, r.state.ProjectID, cfg.AuditID,
		fmt.Sprintf("qa100:%s:r%03d:paused:%d", r.state.RunID, round, nonce),
		"QA_PASS", map[string]any{"round": round, "kind": "PAUSED"}, nil)
	if !errors.As(pausedErr, &apiErr) || (apiErr.Status != 400 && apiErr.Status != 403) {
		failures = append(failures, fmt.Sprintf("R%03d paused audit submit not rejected: %v", round, pausedErr))
	}
	if err := r.client.updateAuditStatus(ctx, cfg.AuditID, "ACTIVE"); err != nil {
		failures = append(failures, fmt.Sprintf("R%03d resume audit: %v", round, err))
	}
	if _, err := r.client.reviewSummary(ctx, r.state.ProjectID); err != nil {
		warnings = append(warnings, fmt.Sprintf("R%03d review summary: %v", round, err))
	}
	return failures, warnings
}

func (r *runner) cleanupRoundRedis(ctx context.Context, round int) {
	pattern := fmt.Sprintf("qa100:%s:r%03d:*", r.state.RunID, round)
	iter := r.actionRedis.Scan(ctx, 0, pattern, 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if len(keys) > 0 {
		_ = r.actionRedis.Del(ctx, keys...).Err()
	}
}

func prefixFailures(round int, label, taskNo string, failures []string) []string {
	out := make([]string, 0, len(failures))
	for _, failure := range failures {
		out = append(out, fmt.Sprintf("R%03d %s %s: %s", round, label, taskNo, failure))
	}
	return out
}

func prefixWarnings(round int, label, taskNo string, warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, fmt.Sprintf("R%03d %s %s: %s", round, label, taskNo, warning))
	}
	return out
}

func routeEntries(detail map[string]any) []map[string]any {
	if detail == nil {
		return nil
	}
	raw, ok := detail["routes"]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func uint64FromAny(v any) uint64 {
	switch value := v.(type) {
	case float64:
		return uint64(value)
	case int:
		return uint64(value)
	case int64:
		return uint64(value)
	case json.Number:
		n, _ := value.Int64()
		return uint64(n)
	default:
		var parsed uint64
		fmt.Sscanf(fmt.Sprint(v), "%d", &parsed)
		return parsed
	}
}

func taskStatus(t *task) string {
	if t == nil {
		return ""
	}
	return t.Status
}

func taskDuration(t *task) int64 {
	if t == nil {
		return 0
	}
	return t.DurationMS
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func cleanupTestProject(ctx context.Context, te *testEnv, c *apiClient) error {
	if te.state.ProjectID == 0 || !strings.HasPrefix(te.state.ProjectCode, "qa100-") {
		return fmt.Errorf("refusing to clean non-QA project %d/%q", te.state.ProjectID, te.state.ProjectCode)
	}
	db, err := openTestDB(te.mysqlDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	var projectCode string
	if err := db.QueryRowContext(ctx, "SELECT code FROM projects WHERE id = ?", te.state.ProjectID).Scan(&projectCode); err != nil {
		return fmt.Errorf("verify cleanup project: %w", err)
	}
	if projectCode != te.state.ProjectCode || !strings.HasPrefix(projectCode, "qa100-") {
		return fmt.Errorf("cleanup project mismatch: %d=%q want %q", te.state.ProjectID, projectCode, te.state.ProjectCode)
	}
	appRedis := redis.NewClient(&redis.Options{Addr: te.redisAddr, Password: te.redisPass, DB: te.redisDB})
	defer appRedis.Close()
	actionRedis := redis.NewClient(&redis.Options{Addr: te.redisAddr, Password: te.redisPass, DB: 14})
	defer actionRedis.Close()
	for _, client := range []*redis.Client{appRedis, actionRedis} {
		patterns := []string{fmt.Sprintf("qa100:%s:*", te.state.RunID)}
		if client == appRedis {
			patterns = append(patterns, fmt.Sprintf("sdv:%d:*", te.state.ProjectID))
		}
		for _, pattern := range patterns {
			iter := client.Scan(ctx, 0, pattern, 200).Iterator()
			var keys []string
			for iter.Next(ctx) {
				keys = append(keys, iter.Val())
			}
			if len(keys) > 0 {
				_ = client.Del(ctx, keys...).Err()
			}
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.columns
		WHERE table_schema = DATABASE() AND column_name = 'project_id'`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}
	rows.Close()
	sort.Strings(tables)
	// Child tables first; projects itself is handled last.
	priorities := map[string]int{
		"script_executions": 1, "task_events": 2, "tasks": 3,
		"trigger_sources": 4, "redis_connections": 5, "ai_providers": 6,
		"provider_usage_logs": 7, "audit_logs": 8, "api_credentials": 9,
		"project_members": 10, "project_secrets": 11, "audits": 12,
	}
	sort.SliceStable(tables, func(i, j int) bool {
		pi, oki := priorities[tables[i]]
		pj, okj := priorities[tables[j]]
		if !oki {
			pi = 100
		}
		if !okj {
			pj = 100
		}
		return pi < pj
	})
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if table == "projects" {
			continue
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s` WHERE project_id = ?", table), te.state.ProjectID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", te.state.ProjectID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete project: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	var remaining int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE project_id = ?", te.state.ProjectID).Scan(&remaining)
	if remaining != 0 {
		return fmt.Errorf("cleanup left %d tasks", remaining)
	}
	var projectCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects WHERE id = ?", te.state.ProjectID).Scan(&projectCount)
	if projectCount != 0 {
		return fmt.Errorf("cleanup left project row")
	}
	return nil
}
