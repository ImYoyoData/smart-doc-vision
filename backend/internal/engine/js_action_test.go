package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
)

func newTestScriptRun() *scriptRun {
	return &scriptRun{
		engine: &Engine{cfg: config.Config{AllowPrivate: true}},
		ctx:    context.Background(),
		cfg: model.Project{
			JSMaxMemoryMB: 64, JSTimeoutMS: 5000,
			JSMaxLogBytes: 32768, JSMaxResultBytes: 65536,
			JSMaxHTTPRequests: 20, JSMaxHTTPResponseBytes: 2 << 20,
			JSMaxDBRows: 100, JSMaxConcurrency: 2,
		},
		render: routeRender{
			Task: model.Task{ID: 7, TaskNo: "SDV-TEST", BizKey: "biz-1", SourceType: model.TriggerOpenAPI},
			Audit: model.Audit{
				ID: 2, Code: "archive_review", Name: "档案审核",
				DecisionMode: model.DecisionReview,
				Categories:   model.JSONBytes([]model.AuditCategory{{Code: "contract", Name: "合同", Result: "PASS"}}),
				MediaTypes:   model.JSONBytes([]string{"text", "image"}),
			},
			Project:     model.Project{ID: 1, Code: "demo", Name: "演示空间"},
			Input:       model.TaskInput{BizKey: "biz-1"},
			FinalResult: model.ResultReject,
			Detail: map[string]any{
				"summary": "缺失印章", "risk_level": "high", "score": 0.9, "category": "contract",
			},
		},
	}
}

func TestJSActionRunsTopLevelCodeWithGlobalContext(t *testing.T) {
	run := newTestScriptRun()
	status, errType, errMsg, output := run.run(`
ctx.log('top-level', ctx.result)
;({
  ok: true,
  code: ctx.audit.code,
  result: ctx.result
})
`, 8<<20)
	if status != model.ScriptSuccess {
		t.Fatalf("status=%s type=%s msg=%s", status, errType, errMsg)
	}
	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("decode output: %v (%s)", err, string(output))
	}
	if payload["ok"] != true || payload["code"] != "archive_review" || payload["result"] != model.ResultReject {
		t.Fatalf("unexpected top-level output: %+v", payload)
	}
	if !strings.Contains(run.logText(), "top-level REJECT") {
		t.Fatalf("logs missing top-level output: %q", run.logText())
	}
}

func TestJSActionTopLevelWithoutReturnValue(t *testing.T) {
	run := newTestScriptRun()
	status, errType, errMsg, output := run.run(`
const rows = [{ id: 1 }]
ctx.log('rows', rows.length)
`, 8<<20)
	if status != model.ScriptSuccess {
		t.Fatalf("status=%s type=%s msg=%s", status, errType, errMsg)
	}
	if len(output) != 0 {
		t.Fatalf("expected no result, got %s", string(output))
	}
}

func TestJSActionRunsWithContextAndLogs(t *testing.T) {
	run := newTestScriptRun()
	status, errType, errMsg, output := run.run(`
function main(ctx) {
  ctx.log('hello', ctx.result)
  return {
    ok: true,
    code: ctx.get('audit.code'),
    same: ctx.audit.code,
    vars: ctx.variables().result,
    catalog: ctx.audit.hasCategory('contract'),
    b64: ctx.crypto.base64Encode('abc'),
    decoded: new Uint8Array(ctx.crypto.base64Decode('aGk='))[0],
    sha: ctx.crypto.sha256('abc'),
    fs: typeof require,
    proc: typeof process,
    timer: typeof setTimeout,
    fetch: typeof fetch
  }
}
`, 8<<20)
	if status != model.ScriptSuccess {
		t.Fatalf("status=%s type=%s msg=%s", status, errType, errMsg)
	}
	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("decode output: %v (%s)", err, string(output))
	}
	if payload["code"] != "archive_review" || payload["same"] != "archive_review" {
		t.Fatalf("unexpected audit context: %+v", payload)
	}
	if payload["vars"] != model.ResultReject || payload["catalog"] != true {
		t.Fatalf("unexpected variables/category: %+v", payload)
	}
	if payload["b64"] != "YWJj" || len(fmtString(payload["sha"])) != 64 {
		t.Fatalf("unexpected crypto values: %+v", payload)
	}
	for _, key := range []string{"fs", "proc", "timer", "fetch"} {
		if payload[key] != "undefined" {
			t.Fatalf("%s should be undefined, got %v", key, payload[key])
		}
	}
	if payload["decoded"] != float64(104) {
		t.Fatalf("binary decode failed: %+v", payload)
	}
	if !strings.Contains(run.logText(), "hello REJECT") {
		t.Fatalf("logs missing script output: %q", run.logText())
	}
}

func TestJSActionHTTPRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "yes")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	run := newTestScriptRun()
	status, errType, errMsg, output := run.run(`
function main(ctx) {
  const res = ctx.http.request({ url: '`+server.URL+`', method: 'GET' })
  return { status: res.status, body: res.body, ok: res.ok }
}
`, 8<<20)
	if status != model.ScriptSuccess {
		t.Fatalf("status=%s type=%s msg=%s", status, errType, errMsg)
	}
	var payload map[string]any
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload["status"] != float64(200) || payload["body"] != "hello" || payload["ok"] != true {
		t.Fatalf("unexpected HTTP result: %+v", payload)
	}
}

func TestJSActionTimeout(t *testing.T) {
	run := newTestScriptRun()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	run.ctx = ctx
	status, errType, _, _ := run.run(`function main(ctx) { while (true) {} }`, 8<<20)
	if status != model.ScriptTimeout || errType != model.ScriptTimeout {
		t.Fatalf("expected timeout, got status=%s type=%s", status, errType)
	}
}

func TestJSActionTopLevelTimeout(t *testing.T) {
	run := newTestScriptRun()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	run.ctx = ctx
	status, errType, _, _ := run.run(`while (true) {}`, 8<<20)
	if status != model.ScriptTimeout || errType != model.ScriptTimeout {
		t.Fatalf("expected top-level timeout, got status=%s type=%s", status, errType)
	}
}

func TestJSActionMemoryLimit(t *testing.T) {
	run := newTestScriptRun()
	status, errType, _, _ := run.run(`
function main(ctx) {
  const keep = []
  while (true) { keep.push(new Array(50000).fill('x')) }
}
`, 512<<10)
	if status != model.ScriptOOM || errType != model.ScriptOOM {
		t.Fatalf("expected OOM, got status=%s type=%s", status, errType)
	}
}

func fmtString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func TestMySQLDSNAllowsNativePassword(t *testing.T) {
	run := newTestScriptRun()
	dsn, err := run.mysqlDSN(map[string]any{
		"host": "127.0.0.1", "user": "testacc", "password": "testacc", "database": "TestDb",
	})
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if !parsed.AllowNativePasswords {
		t.Fatalf("DSN must enable native password auth for MySQL 5.7: %s", dsn)
	}
}
