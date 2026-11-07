package api

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"smartdoc-vision/backend/internal/ai"
)

// sessionContext tags the request context with the chat ID supplied by the
// browser. Every model call made while handling this chat reuses it, so an
// upstream gateway sees one conversation instead of a stream of unrelated
// requests. A new chat in the UI sends a new ID.
func sessionContext(c *gin.Context, sessionID string) context.Context {
	return ai.WithSessionID(c.Request.Context(), strings.TrimSpace(sessionID))
}
