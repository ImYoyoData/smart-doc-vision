// Package httpx holds the process-wide outbound HTTP transport.
//
// Creating an http.Client without a Transport gives it a private connection
// pool: every call keeps its own idle sockets and read/write buffers until the
// garbage collector happens to reclaim them. Reusing one transport bounds that
// memory and lets webhooks, callbacks and model calls share keep-alive
// connections.
package httpx

import (
	"net/http"
	"time"
)

var shared = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	return transport
}()

// Transport is the shared outbound transport. Callers should not modify it.
func Transport() *http.Transport { return shared }

// Client returns a client with the given timeout that shares Transport.
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: shared}
}
