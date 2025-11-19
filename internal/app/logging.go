package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"live-stream-alerts/internal/logging"
)

func configureLogging(logPath string) (*os.File, error) {
	started := time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	if err := rotateExistingLog(logPath, started); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	logging.SetDefaultWriter(io.MultiWriter(os.Stdout, file))
	return file, nil
}

func rotateExistingLog(logPath string, started time.Time) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat log file: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}

	logDir := filepath.Dir(logPath)
	archiveDir := filepath.Join(logDir, "logs")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("create log archive dir: %w", err)
	}

	baseTimestamp := started.Format("2006-01-02_15-04-05")
	baseName := fmt.Sprintf("alertserver-%s.log", baseTimestamp)
	destPath := filepath.Join(archiveDir, baseName)
	for i := 1; ; i++ {
		if _, err := os.Stat(destPath); errors.Is(err, os.ErrNotExist) {
			break
		}
		destPath = filepath.Join(archiveDir, fmt.Sprintf("alertserver-%s-%d.log", baseTimestamp, i))
	}
	if err := os.Rename(logPath, destPath); err != nil {
		return fmt.Errorf("archive log file: %w", err)
	}
	return nil
}
