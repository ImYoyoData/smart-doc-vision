package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type apiResponse struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
	CostMS  int64  `json:"cost_ms,omitempty"`
}

func ok(c *gin.Context, data any) {
	respond(c, http.StatusOK, 0, "OK", data)
}

func created(c *gin.Context, data any) {
	respond(c, http.StatusCreated, 0, "OK", data)
}

func fail(c *gin.Context, status int, code, message string, data any) {
	respond(c, status, code, message, data)
}

func respond(c *gin.Context, status int, code any, message string, data any) {
	start, _ := c.Get("started_at")
	elapsed := int64(0)
	if t, ok := start.(time.Time); ok {
		elapsed = time.Since(t).Milliseconds()
	}
	c.JSON(status, apiResponse{
		Code:    code,
		Message: message,
		Data:    data,
		TraceID: c.GetString("trace_id"),
		CostMS:  elapsed,
	})
}

func badRequest(c *gin.Context, message string) {
	fail(c, http.StatusBadRequest, "INVALID_PARAM", message, nil)
}

func notFound(c *gin.Context, message string) {
	fail(c, http.StatusNotFound, "NOT_FOUND", message, nil)
}

func serverError(c *gin.Context, err error) {
	fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
}

func forbidden(c *gin.Context, message string) {
	fail(c, http.StatusForbidden, "PERM_DENIED", message, nil)
}

func conflict(c *gin.Context, code, message string) {
	fail(c, http.StatusConflict, code, message, nil)
}
