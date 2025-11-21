// Package logging centralises the alert-server logging helpers and adapters.
package logging

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
)

// Logger represents the minimal logging interface used across the project.
type Logger interface {
	Printf(format string, v ...any)
}

// StdLoggerProvider can expose the underlying *log.Logger when needed.
type stdLoggerProvider interface {
	StdLogger() *log.Logger
}

type stdLogger struct {
	base *log.Logger
}

type newlineWriter struct {
	mu sync.Mutex
	w  io.Writer
}

var (
	defaultWriter   io.Writer = os.Stdout
	defaultWriterMu sync.RWMutex
)

const maxLoggedResponseBody = 4096

// New returns a Logger that writes to stdout using Go's default date/time flags.
func New() Logger {
	return NewWithWriter(getDefaultWriter())
}

// NewWithWriter builds a Logger that writes to the provided io.Writer and
// ensures there is always a blank line before each timestamped entry.
func NewWithWriter(w io.Writer) Logger {
	if w == nil {
		w = os.Stdout
	}
	adapter := &newlineWriter{w: w}
	return &stdLogger{base: log.New(adapter, "", log.LstdFlags)}
}

// SetDefaultWriter overrides the writer used by New().
func SetDefaultWriter(w io.Writer) {
	defaultWriterMu.Lock()
	defer defaultWriterMu.Unlock()
	if w == nil {
		defaultWriter = os.Stdout
		return
	}
	defaultWriter = w
}

func getDefaultWriter() io.Writer {
	defaultWriterMu.RLock()
	defer defaultWriterMu.RUnlock()
	return defaultWriter
}

// AsStdLogger returns the underlying *log.Logger when available so packages
// like net/http can keep using their native logger type.
func AsStdLogger(logger Logger) *log.Logger {
	if logger == nil {
		return nil
	}
	if provider, ok := logger.(stdLoggerProvider); ok {
		return provider.StdLogger()
	}
	if std, ok := logger.(*stdLogger); ok {
		return std.base
	}
	return nil
}

func (l *stdLogger) Printf(format string, v ...any) {
	if l == nil || l.base == nil {
		return
	}
	l.base.Printf(format, v...)
}

func (l *stdLogger) StdLogger() *log.Logger {
	if l == nil {
		return nil
	}
	return l.base
}

func (w *newlineWriter) Write(p []byte) (int, error) {
	if w == nil || w.w == nil {
		return len(p), nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.w.Write([]byte("\n")); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := w.w.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WithHTTPLogging wraps the provided handler so every request/response pair is logged.
func WithHTTPLogging(next http.Handler, logger Logger) http.Handler {
	if logger == nil || next == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dump, err := httputil.DumpRequest(r, true); err == nil {
			logger.Printf("---- Incoming request from %s ----\n%s", r.RemoteAddr, dump)
		} else {
			logger.Printf("failed to dump request from %s: %v", r.RemoteAddr, err)
		}

		lrw := newLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)

		status := lrw.StatusCode()
		logger.Printf(
			"---- Response for %s %s (%d %s) ----\n%s",
			r.Method,
			r.URL.Path,
			status,
			http.StatusText(status),
			lrw.LoggedBody(),
		)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status    int
	buf       bytes.Buffer
	truncated bool
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{ResponseWriter: w}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.status == 0 {
		lrw.status = http.StatusOK
	}
	if lrw.buf.Len() < maxLoggedResponseBody {
		remaining := maxLoggedResponseBody - lrw.buf.Len()
		if len(b) > remaining {
			lrw.buf.Write(b[:remaining])
			lrw.truncated = true
		} else {
			lrw.buf.Write(b)
		}
	} else {
		lrw.truncated = true
	}
	return lrw.ResponseWriter.Write(b)
}

func (lrw *loggingResponseWriter) StatusCode() int {
	if lrw.status == 0 {
		return http.StatusOK
	}
	return lrw.status
}

func (lrw *loggingResponseWriter) LoggedBody() string {
	body := lrw.buf.String()
	if lrw.truncated {
		return fmt.Sprintf("%s\n-- response truncated after %d bytes --", body, maxLoggedResponseBody)
	}
	return body
}

func (lrw *loggingResponseWriter) Flush() {
	if flusher, ok := lrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
