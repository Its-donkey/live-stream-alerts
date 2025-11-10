// internal/logging/logger.go
package logging

type Logger interface {
	Printf(format string, v ...any)
}
