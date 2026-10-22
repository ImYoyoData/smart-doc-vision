package trigger

import (
	"testing"
	"time"

	"smartdoc-vision/backend/internal/model"
)

func TestFingerprintIgnoresUpdatedAt(t *testing.T) {
	base := model.TriggerSource{
		ID: 3, Type: model.TriggerRedis, Status: "ACTIVE",
		ConfigJSON: model.JSONText(`{"stream":"sdv:demo:audit:review","group":"sdv-cg-3"}`),
	}
	first := base
	first.UpdatedAt = time.Now()
	second := base
	second.UpdatedAt = time.Now().Add(time.Hour)
	if fingerprint(first) != fingerprint(second) {
		t.Fatalf("heartbeat updates must not recycle the worker: %q != %q", fingerprint(first), fingerprint(second))
	}

	changed := base
	changed.ConfigJSON = model.JSONText(`{"stream":"sdv:demo:audit:review","group":"sdv-cg-3","batch_size":8}`)
	if fingerprint(first) == fingerprint(changed) {
		t.Fatal("config changes must restart the worker")
	}
}
