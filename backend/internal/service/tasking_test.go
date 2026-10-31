package service

import (
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestRequestFingerprintIgnoresCorrelationFields(t *testing.T) {
	input := model.TaskInput{
		BizKey: "archive:1",
		Media:  []model.MediaItem{{Type: "text", Content: "hello"}},
		Ext:    map[string]any{"scene": "archive"},
	}
	first := requestFingerprint(10, model.TriggerOpenAPI, 20, input)
	input.RequestID = "request-1"
	input.SourceRef = "message-1"
	input.CallbackURL = "https://example.com/callback"
	if second := requestFingerprint(10, model.TriggerOpenAPI, 20, input); second != first {
		t.Fatal("request_id/source_ref/callback_url must not change the idempotency fingerprint")
	}
	input.Media[0].Content = "changed"
	if changed := requestFingerprint(10, model.TriggerOpenAPI, 20, input); changed == first {
		t.Fatal("changed media must change the idempotency fingerprint")
	}
}

func TestDedupeHashIncludesRequestID(t *testing.T) {
	input := model.TaskInput{
		BizKey: "archive:1",
		Media:  []model.MediaItem{{Type: "text", Content: "hello"}},
	}
	first, _, _ := dedupeHash(1, 2, model.TriggerOpenAPI, 3, input, 3600)
	input.RequestID = "request-1"
	second, _, _ := dedupeHash(1, 2, model.TriggerOpenAPI, 3, input, 3600)
	if first == second {
		t.Fatal("request_id must make the stored dedupe hash unique")
	}
}
