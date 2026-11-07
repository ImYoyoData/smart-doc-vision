package ai

import (
	"net/http"
	"time"

	"smartdoc-vision/backend/internal/httpx"
)

// clientWithTimeout returns a client sharing the process-wide outbound
// transport. A fresh http.Client per call would own its own idle connection
// pool, keeping sockets and buffers alive after the request finished.
func clientWithTimeout(timeout time.Duration) *http.Client {
	return httpx.Client(timeout)
}
