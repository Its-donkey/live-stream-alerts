// internal/logging/logger.go
package logging

import (
	"io"
	"log"
	"os"
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

// New returns a Logger that writes to stdout using Go's default date/time flags.
func New() Logger {
	return NewWithWriter(os.Stdout)
}

// NewWithWriter builds a Logger that writes to the provided io.Writer.
func NewWithWriter(w io.Writer) Logger {
	if w == nil {
		w = os.Stdout
	}
	return &stdLogger{base: log.New(w, "", log.LstdFlags)}
}

// FromStd wraps an existing *log.Logger in the Logger interface.
func FromStd(base *log.Logger) Logger {
	if base == nil {
		return New()
	}
	return &stdLogger{base: base}
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
