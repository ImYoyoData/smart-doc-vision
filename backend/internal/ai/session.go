package ai

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// SessionHeader is the outbound header carrying the conversation ID. Gateways
// use it to group requests that belong to one chat; it replaces the older
// provider-specific x-opencode-session header.
const SessionHeader = "X-Session-Id"

// legacySessionHeader is still honoured when an administrator writes it in the
// provider's custom headers.
const legacySessionHeader = "x-opencode-session"

type sessionKey struct{}

// WithSessionID tags a context with the conversation ID that outbound model
// calls should send. One chat keeps the same ID for all of its turns; a new
// chat gets a new random ID.
func WithSessionID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, id)
}

// SessionID returns the conversation ID stored in ctx, if any.
func SessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(sessionKey{}).(string)
	return strings.TrimSpace(id)
}

// NewSessionID returns a fresh random conversation ID.
func NewSessionID() string { return uuid.NewString() }

// headerSessionID resolves the ID to send: the caller-provided one first, then
// the one tagged on the context, then a fresh UUID.
func headerSessionID(ctx context.Context, explicit map[string]string) string {
	for key, value := range explicit {
		if strings.EqualFold(strings.TrimSpace(key), SessionHeader) || strings.EqualFold(strings.TrimSpace(key), legacySessionHeader) {
			if trimmed := strings.TrimSpace(value); trimmed != "" && !isRandomPlaceholder(trimmed) {
				return trimmed
			}
		}
	}
	if id := SessionID(ctx); id != "" {
		return id
	}
	return NewSessionID()
}

func isRandomPlaceholder(value string) bool {
	return strings.EqualFold(value, "{{random}}") || strings.EqualFold(value, "auto")
}
