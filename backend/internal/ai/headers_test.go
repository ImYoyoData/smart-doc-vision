package ai

import (
	"net/http"
	"strings"
	"testing"

	"smartdoc-vision/backend/internal/model"
)

func TestApplyCustomHeadersAddsSessionHeaderByDefault(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(req, nil)
	if session := req.Header.Get("x-opencode-session"); session == "" {
		t.Fatal("expected an automatically generated x-opencode-session header")
	}
}

func TestApplyCustomHeadersExpandsRandomPlaceholder(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	req.Header.Set("Authorization", "Bearer secret")
	ApplyCustomHeaders(req, map[string]string{
		"x-opencode-session": "{{random}}",
		"Content-Type":       "text/plain",
	})
	session := req.Header.Get("x-opencode-session")
	if session == "" || strings.Contains(session, "{{random}}") {
		t.Fatalf("placeholder was not replaced: %q", session)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("authorization header was overwritten: %q", got)
	}
}

func TestApplyCustomHeadersRespectsFixedSession(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", nil)
	ApplyCustomHeaders(req, map[string]string{"x-opencode-session": "fixed-session"})
	if got := req.Header.Get("x-opencode-session"); got != "fixed-session" {
		t.Fatalf("fixed session header = %q", got)
	}
}

func TestWithRandomSessionHeaderReplacesFixedValue(t *testing.T) {
	provider := model.AIProvider{ConfigJSON: model.JSONBytes(model.ProviderConfig{
		Headers: map[string]string{"x-opencode-session": "fixed-session"},
	})}
	updated := WithRandomSessionHeader(provider)
	var cfg model.ProviderConfig
	if err := model.DecodeJSON(updated.ConfigJSON, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	session := cfg.Headers["x-opencode-session"]
	if session == "" || session == "fixed-session" {
		t.Fatalf("session header was not randomized: %q", session)
	}
}
