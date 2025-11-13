package logging

import (
	"bytes"
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
