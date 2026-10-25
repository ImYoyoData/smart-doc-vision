package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/config"
	"smartdoc-vision/backend/internal/model"
)

func TestSimulateScriptMocksExternalIO(t *testing.T) {
	eng := &Engine{cfg: config.Config{}}
	project := model.Project{
		ID: 1, Code: "demo", Name: "演示空间",
		JSMaxMemoryMB: 64, JSTimeoutMS: 5000,
		JSMaxLogBytes: 32768, JSMaxResultBytes: 65536,
		JSMaxHTTPRequests: 20, JSMaxHTTPResponseBytes: 2 << 20,
		JSMaxDBRows: 100, JSMaxConcurrency: 2,
	}
	result := eng.SimulateScript(context.Background(), ScriptSimulationOptions{
		Project: project,
		Audit:   model.Audit{ID: 2, ProjectID: 1, Code: "archive_review", Name: "档案审核", DecisionMode: model.DecisionReview},
		Task:    model.Task{ID: 1, TaskNo: "SIM-001", BizKey: "biz-1", SourceType: model.TriggerOpenAPI, Status: model.TaskSuccess},
		Input:   model.TaskInput{BizKey: "biz-1", Media: []model.MediaItem{{Type: "text", Content: "模拟输入"}}},
		Result:  model.ResultReject,
		Detail:  map[string]any{"summary": "模拟驳回", "risk_level": "high", "score": 0.9},
		Script: `
const db = ctx.mysql.connect({ host: '127.0.0.1', port: 3306, user: 'u', password: 'p', database: 'd' })
const rows = db.query('select 1')
const exec = db.exec('update t set x = 1')
const tx = db.begin()
tx.rollback()
const http = ctx.http.request({ url: 'https://example.com/hook', method: 'POST' })
const secret = ctx.secret('TOKEN')
;({
  rows: rows.length,
  affected: exec.rows_affected,
  http: http.status,
  secret,
  result: ctx.result,
  simulation: ctx.simulation,
  taskNo: ctx.variables().task_no
})
`,
	})
	if result.Status != model.ScriptSuccess {
		t.Fatalf("status=%s type=%s error=%s logs=%v", result.Status, result.ErrorType, result.ErrorMessage, result.Logs)
	}
	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("decode output: %v (%s)", err, string(result.Output))
	}
	if output["rows"] != float64(0) || output["affected"] != float64(0) {
		t.Fatalf("unexpected DB mocks: %#v", output)
	}
	if output["http"] != float64(200) || output["secret"] != "simulated-secret" {
		t.Fatalf("unexpected HTTP/secret mocks: %#v", output)
	}
	if output["result"] != model.ResultReject || output["simulation"] != true || output["taskNo"] != "SIM-001" {
		t.Fatalf("unexpected context: %#v", output)
	}
	logs := strings.Join(result.Logs, "\n")
	for _, expected := range []string{"模拟运行开始", "不连接 MySQL", "拦截 HTTP POST", "ctx.secret"} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("logs missing %q:\n%s", expected, logs)
		}
	}
	if result.Variables["result"] != model.ResultReject {
		t.Fatalf("unexpected variables: %#v", result.Variables)
	}
}
