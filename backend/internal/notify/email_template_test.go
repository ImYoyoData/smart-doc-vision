package notify

import (
	"strings"
	"testing"
	"time"

	"smartdoc-vision/backend/internal/model"
)

func TestRenderMessageEmailEscapesContent(t *testing.T) {
	requestID := "req-<test>"
	task := model.Task{
		TaskNo: "SDV-1", SourceType: model.TriggerOpenAPI, BizKey: `biz&key`,
		RequestID: &requestID, Status: model.TaskReviewPending, CreatedAt: time.Date(2026, 10, 31, 10, 0, 0, 0, time.Local),
	}
	audit := model.Audit{Name: "合同审核", Code: "contract"}
	project := model.Project{Name: "档案项目", Code: "archive"}
	message := model.InAppMessage{
		Title: "待议复核 · 合同审核", Body: `模型摘要 <script>alert(1)</script>`,
		Link: "/reviews?task_id=1", TaskID: 1, ProjectID: 1, AuditID: 1,
	}
	text, html, err := RenderMessageEmail(buildMessageEmailData("Acme", "https://review.example.com", message, task, audit, project))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "https://review.example.com/reviews?task_id=1") {
		t.Fatal("action URL missing")
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("HTML content was not escaped")
	}
	if !strings.Contains(text, "SDV-1") || !strings.Contains(text, "biz&key") {
		t.Fatalf("text fallback is missing details: %s", text)
	}
}
