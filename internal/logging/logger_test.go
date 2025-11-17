package logging

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewWithWriterInsertsLeadingNewline(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf)
	logger.Printf("hello %s", "world")
	got := buf.String()
	if got == "" || got[0] != '\n' {
		t.Fatalf("expected leading newline, got %q", got)
	}
	if !bytes.Contains(buf.Bytes(), []byte("hello world")) {
		t.Fatalf("expected log body to contain message, got %q", got)
	}
}

func TestSetDefaultWriterAffectsNew(t *testing.T) {
	var buf bytes.Buffer
	SetDefaultWriter(&buf)
	t.Cleanup(func() { SetDefaultWriter(os.Stdout) })
	logger := New()
	logger.Printf("captured")
	if !strings.Contains(buf.String(), "captured") {
		t.Fatalf("expected log output to be written to buffer, got %q", buf.String())
	}
}

func TestAsStdLoggerReturnsUnderlyingLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf)
	std := AsStdLogger(logger)
	if std == nil {
		t.Fatalf("expected std logger")
	}
	std.Print("testing")
	if !bytes.Contains(buf.Bytes(), []byte("testing")) {
		t.Fatalf("expected std logger to write to buffer")
	}
}

func TestAsStdLoggerNilSafe(t *testing.T) {
	if got := AsStdLogger(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	var l *stdLogger
	if got := AsStdLogger(l); got != nil {
		t.Fatalf("expected nil for nil receiver")
	}
}

func TestStdLoggerPrintFSafe(t *testing.T) {
	var l *stdLogger
	l.Printf("ignore")
}

func TestStdLoggerStdLoggerSafe(t *testing.T) {
	sl := &stdLogger{}
	if sl.StdLogger() != nil {
		t.Fatalf("expected nil base")
	}
}

type captureLogger struct {
	entries []string
}

func (c *captureLogger) Printf(format string, args ...any) {
	c.entries = append(c.entries, fmt.Sprintf(format, args...))
}

func TestWithHTTPLoggingWrapsHandler(t *testing.T) {
	logger := &captureLogger{}
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("payload"))
	})
	handler := WithHTTPLogging(base, logger)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	if len(logger.entries) < 2 {
		t.Fatalf("expected request/response logs to be recorded")
	}
}

func TestWithHTTPLoggingNilLoggerReturnsOriginal(t *testing.T) {
	base := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if got := WithHTTPLogging(base, nil); fmt.Sprintf("%p", got) != fmt.Sprintf("%p", base) {
		t.Fatalf("expected handler to be returned untouched when logger is nil")
	}
}

func TestLoggingResponseWriterTruncatesLargeBodies(t *testing.T) {
	rr := httptest.NewRecorder()
	lrw := newLoggingResponseWriter(rr)
	payload := strings.Repeat("x", maxLoggedResponseBody+10)

	if _, err := lrw.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if lrw.StatusCode() != http.StatusOK {
		t.Fatalf("expected default status to be 200, got %d", lrw.StatusCode())
	}
	body := lrw.LoggedBody()
	if !strings.Contains(body, "-- response truncated after") {
		t.Fatalf("expected truncation notice, got %q", body)
	}
}

type flushRecorder struct {
	http.ResponseWriter
	flushed bool
}

func (f *flushRecorder) Flush() {
	f.flushed = true
}

func TestLoggingResponseWriterImplementsFlusher(t *testing.T) {
	fr := &flushRecorder{ResponseWriter: httptest.NewRecorder()}
	lrw := newLoggingResponseWriter(fr)
	flusher, ok := interface{}(lrw).(http.Flusher)
	if !ok {
		t.Fatalf("expected loggingResponseWriter to implement http.Flusher")
	}
	flusher.Flush()
	if !fr.flushed {
		t.Fatalf("expected underlying flusher to be invoked")
	}
}
