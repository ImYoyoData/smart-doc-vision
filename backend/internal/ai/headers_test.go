package ai

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestApplyCustomHeadersAddsSessionHeaderByDefault(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(context.Background(), req, nil)
	if session := req.Header.Get(SessionHeader); session == "" {
		t.Fatalf("expected an automatically generated %s header", SessionHeader)
	}
	if legacy := req.Header.Get(legacySessionHeader); legacy != "" {
		t.Fatalf("legacy header must not be sent by default, got %q", legacy)
	}
}

func TestApplyCustomHeadersReusesContextSession(t *testing.T) {
	ctx := WithSessionID(context.Background(), "chat-123")
	first, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	second, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(ctx, first, nil)
	ApplyCustomHeaders(ctx, second, nil)
	if got := first.Header.Get(SessionHeader); got != "chat-123" {
		t.Fatalf("first request session = %q", got)
	}
	if got := second.Header.Get(SessionHeader); got != "chat-123" {
		t.Fatalf("same chat must keep one session id, got %q", got)
	}
}

func TestApplyCustomHeadersGeneratesNewSessionPerContext(t *testing.T) {
	first, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	second, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(context.Background(), first, nil)
	ApplyCustomHeaders(context.Background(), second, nil)
	if first.Header.Get(SessionHeader) == second.Header.Get(SessionHeader) {
		t.Fatal("separate chats must not share a session id")
	}
}

func TestApplyCustomHeadersExpandsRandomPlaceholder(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	req.Header.Set("Authorization", "Bearer secret")
	ApplyCustomHeaders(context.Background(), req, map[string]string{
		"X-Agent":      "{{random}}",
		"Content-Type": "text/plain",
	})
	value := req.Header.Get("X-Agent")
	if value == "" || strings.Contains(value, "{{random}}") {
		t.Fatalf("placeholder was not replaced: %q", value)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("authorization header was overwritten: %q", got)
	}
	if got := req.Header.Get("Content-Type"); got == "text/plain" {
		t.Fatal("content type must not be overridden by custom headers")
	}
}

func TestApplyCustomHeadersRespectsFixedSession(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(context.Background(), req, map[string]string{SessionHeader: "fixed-session"})
	if got := req.Header.Get(SessionHeader); got != "fixed-session" {
		t.Fatalf("fixed session header = %q", got)
	}
}

func TestApplyCustomHeadersHonoursLegacySessionKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(context.Background(), req, map[string]string{legacySessionHeader: "legacy-chat"})
	if got := req.Header.Get(SessionHeader); got != "legacy-chat" {
		t.Fatalf("legacy key should map onto %s, got %q", SessionHeader, got)
	}
	if legacy := req.Header.Get(legacySessionHeader); legacy != "" {
		t.Fatalf("legacy header should not be sent, got %q", legacy)
	}
}

func TestWithRandomSessionHeaderDropsFixedValue(t *testing.T) {
	provider := model.AIProvider{ConfigJSON: model.JSONBytes(model.ProviderConfig{
		Headers: map[string]string{SessionHeader: "fixed-session"},
	})}
	updated := WithRandomSessionHeader(provider)
	var cfg model.ProviderConfig
	if err := model.DecodeJSON(updated.ConfigJSON, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if _, exists := cfg.Headers[SessionHeader]; exists {
		t.Fatal("health probes must not reuse the configured chat id")
	}
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(context.Background(), req, cfg.Headers)
	if req.Header.Get(SessionHeader) == "" {
		t.Fatal("probe still needs a random session id")
	}
}

func TestSessionIDRoundTrip(t *testing.T) {
	if got := SessionID(context.Background()); got != "" {
		t.Fatalf("empty context should have no session, got %q", got)
	}
	ctx := WithSessionID(context.Background(), "  abc  ")
	if got := SessionID(ctx); got != "abc" {
		t.Fatalf("session id = %q", got)
	}
}
