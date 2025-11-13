// internal/logging/logger.go
package logging

import (
	"io"
	"log"
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

// New returns a Logger that writes to stdout using Go's default date/time flags.
func New() Logger {
	return NewWithWriter(os.Stdout)
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
