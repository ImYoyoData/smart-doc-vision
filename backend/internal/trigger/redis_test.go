package trigger

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestParseRedisInputMergesTopLevelCorrelation(t *testing.T) {
	msg := redis.XMessage{
		ID: "1793440000000-0",
		Values: map[string]interface{}{
			"request_id":   "audit-1",
			"callback_url": "https://example.com/callback",
			"data":         `{"biz_key":"archive:1","request_id":"inner-id","media":[{"type":"text","content":"hello"}],"ext":{"scene":"archive"}}`,
		},
	}
	input, err := parseRedisInput(msg)
	if err != nil {
		t.Fatal(err)
	}
	if input.RequestID != "audit-1" {
		t.Fatalf("top-level request_id should win: %+v", input)
	}
	if input.CallbackURL != "https://example.com/callback" || input.BizKey != "archive:1" {
		t.Fatalf("top-level callback/biz_key merge failed: %+v", input)
	}
	if len(input.Media) != 1 || input.Media[0].Content != "hello" {
		t.Fatalf("media was not parsed: %+v", input.Media)
	}
}

func TestParseRedisInputKeepsCorrelationWhenDataIsInvalid(t *testing.T) {
	msg := redis.XMessage{
		ID: "1793440000000-1",
		Values: map[string]interface{}{
			"request_id":   "audit-2",
			"callback_url": "https://example.com/callback",
			"data":         `{not-json`,
		},
	}
	input, err := parseRedisInput(msg)
	if err == nil {
		t.Fatal("expected invalid data to fail")
	}
	if input.RequestID != "audit-2" || input.CallbackURL == "" {
		t.Fatalf("correlation fields must survive parse failure: %+v", input)
	}
}
