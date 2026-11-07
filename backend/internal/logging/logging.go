// Package logging wires the application's request and error logs into
// rolling files under <workdir>/logs, and exposes two zerolog loggers.
//
// Logs are rotated when the file grows past 100MB or the date changes, then
// compressed with zstd using a small memory footprint. Only the 30 newest
// archives in each directory are kept.
package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

const (
	logDirName   = "logs"
	infoDirName  = "info"
	errorDirName = "error"
)

var (
	mu          sync.RWMutex
	infoLogger  = newTextLogger(os.Stdout)
	errorLogger = newTextLogger(os.Stderr)
	infoSink    *rollingFile
	errorSink   *rollingFile
)

func newTextLogger(out io.Writer) zerolog.Logger {
	writer := zerolog.ConsoleWriter{
		Out:        out,
		NoColor:    true,
		TimeFormat: "2006-01-02 15:04:05.000",
	}
	return zerolog.New(writer).With().Timestamp().Logger()
}

// Setup creates logs/info and logs/error in the working directory and routes
// both zerolog and the standard library logger to them. If it fails the
// application keeps running with console-only logging and the caller should
// report the error.
func Setup() error {
	info, err := newRollingFile(filepath.Join(logDirName, infoDirName), infoDirName, defaultMaxBytes, defaultMaxBackups)
	if err != nil {
		return err
	}
	errSink, err := newRollingFile(filepath.Join(logDirName, errorDirName), errorDirName, defaultMaxBytes, defaultMaxBackups)
	if err != nil {
		_ = info.Close()
		return err
	}

	mu.Lock()
	infoSink = info
	errorSink = errSink
	infoLogger = newTextLogger(info)
	errorLogger = newTextLogger(io.MultiWriter(os.Stderr, errSink))
	mu.Unlock()

	// Existing diagnostics use the standard library logger; capture them in
	// the error stream alongside stderr.
	log.SetOutput(io.MultiWriter(os.Stderr, errSink))
	return nil
}

// Close flushes and closes the file sinks, compressing archives still queued,
// and restores console-only logging.
func Close() {
	mu.Lock()
	info, errSink := infoSink, errorSink
	infoSink, errorSink = nil, nil
	infoLogger = newTextLogger(os.Stdout)
	errorLogger = newTextLogger(os.Stderr)
	mu.Unlock()
	log.SetOutput(os.Stderr)
	if info != nil {
		_ = info.Close()
	}
	if errSink != nil {
		_ = errSink.Close()
	}
}

// Info logs informational messages such as HTTP requests to logs/info.
func Info() *zerolog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return &infoLogger
}

// Error logs failures and diagnostics to logs/error (and stderr).
func Error() *zerolog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return &errorLogger
}

// RequestLogger records one line per HTTP request in the info log. Requests
// finishing with 5xx are duplicated into the error log.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		latency := time.Since(started)
		status := c.Writer.Status()

		info := requestEvent(Info().Info(), c, status, latency)
		info.Msg("request")
		if status >= 500 {
			failure := requestEvent(Error().Error(), c, status, latency)
			failure.Msg("request failed")
		}
	}
}

func requestEvent(event *zerolog.Event, c *gin.Context, status int, latency time.Duration) *zerolog.Event {
	event = event.
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Int("status", status).
		Str("latency", latency.Round(time.Microsecond).String()).
		Str("ip", c.ClientIP())
	if query := c.Request.URL.RawQuery; query != "" {
		event = event.Str("query", query)
	}
	if traceID := c.GetString("trace_id"); traceID != "" {
		event = event.Str("trace", traceID)
	}
	if size := c.Writer.Size(); size > 0 {
		event = event.Int("size", size)
	}
	return event
}

// Recovery replaces gin.Recovery, writing the panic and its stack trace to the
// error log before gin returns 500.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		Error().Error().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Str("ip", c.ClientIP()).
			Str("trace", c.GetString("trace_id")).
			Interface("panic", recovered).
			Msg("panic recovered")
		Error().Error().Bytes("stack", debug.Stack()).Msg("panic stack")
	})
}
