package engine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Calcium-Ion/moejs"

	"smartdoc-vision/backend/internal/model"
)

type ScriptSimulationOptions struct {
	Project   model.Project
	Audit     model.Audit
	Task      model.Task
	Input     model.TaskInput
	Result    string
	Detail    map[string]any
	Script    string
	MemoryMB  int
	TimeoutMS int
}

type ScriptSimulationResult struct {
	Status       string          `json:"status"`
	ErrorType    string          `json:"error_type,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	DurationMS   int64           `json:"duration_ms"`
	Logs         []string        `json:"logs"`
	Output       json.RawMessage `json:"output,omitempty"`
	Variables    map[string]any  `json:"variables"`
	MemoryBytes  int64           `json:"memory_bytes"`
	TimeoutMS    int             `json:"timeout_ms"`
	MemoryMB     int             `json:"memory_mb"`
}

// SimulateScript runs a result action in a side-effect-free sandbox. The
// script receives a synthetic task/audit context; database, HTTP and secret
// access are replaced by deterministic stubs and logged for inspection.
func (e *Engine) SimulateScript(ctx context.Context, opts ScriptSimulationOptions) ScriptSimulationResult {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := opts.Project
	memoryMB := opts.MemoryMB
	if memoryMB <= 0 || memoryMB > cfg.JSMaxMemoryMB {
		memoryMB = cfg.JSMaxMemoryMB
	}
	if memoryMB <= 0 {
		memoryMB = 64
	}
	timeoutMS := opts.TimeoutMS
	if timeoutMS <= 0 || timeoutMS > cfg.JSTimeoutMS {
		timeoutMS = cfg.JSTimeoutMS
	}
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	result := ScriptSimulationResult{
		Status: model.ScriptSuccess, Logs: []string{}, Variables: map[string]any{},
		TimeoutMS: timeoutMS, MemoryMB: memoryMB,
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	release, err := e.acquireScriptSlot(runCtx, opts.Project.ID, cfg.JSMaxConcurrency)
	if err != nil {
		result.Status = model.ScriptTimeout
		result.ErrorType = model.ScriptTimeout
		result.ErrorMessage = "等待模拟运行槽位超时"
		return result
	}
	defer release()

	detail := opts.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	run := &scriptRun{
		engine: e, ctx: runCtx, cfg: cfg, scriptHash: scriptHash(opts.Script),
		secrets: map[string]string{}, simulate: true,
		render: routeRender{
			Task: opts.Task, Audit: opts.Audit, Project: opts.Project,
			Input: opts.Input, FinalResult: opts.Result, Detail: detail,
		},
	}
	run.logf("SIM", "模拟运行开始：result=%s，ctx.simulation=true；数据库/HTTP/密钥均为模拟", opts.Result)
	started := time.Now()
	status, errType, errMsg, output := run.run(opts.Script, int64(memoryMB)<<20)
	result.DurationMS = time.Since(started).Milliseconds()
	result.Status = status
	result.ErrorType = errType
	result.ErrorMessage = errMsg
	result.MemoryBytes = run.memoryBytes
	result.Logs = splitSimulationLogs(run.logText())
	result.Variables = run.variables()
	if len(output) > 0 && string(output) != "null" {
		result.Output = json.RawMessage(output)
	}
	run.closeAll()
	return result
}

func splitSimulationLogs(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return []string{"模拟运行没有产生日志"}
	}
	return strings.Split(text, "\n")
}

func (r *scriptRun) simulatedDatabase(realm *moejs.Realm) (moejs.Value, error) {
	noop := func(_ *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		return moejs.Undefined(), nil
	}
	query := func(realm *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		return realm.FromGo([]map[string]any{})
	}
	exec := func(realm *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		return realm.FromGo(map[string]any{"rows_affected": 0, "last_insert_id": 0})
	}
	begin := func(realm *moejs.Realm, _ moejs.Value, _ []moejs.Value) (moejs.Value, error) {
		return realm.FromGo(map[string]any{
			"query":    moejs.NativeFunc(query),
			"exec":     moejs.NativeFunc(exec),
			"commit":   moejs.NativeFunc(noop),
			"rollback": moejs.NativeFunc(noop),
		})
	}
	return realm.FromGo(map[string]any{
		"query":  moejs.NativeFunc(query),
		"exec":   moejs.NativeFunc(exec),
		"begin":  moejs.NativeFunc(begin),
		"close":  moejs.NativeFunc(noop),
		"commit": moejs.NativeFunc(noop),
	})
}
