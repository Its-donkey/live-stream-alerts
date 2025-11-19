package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"live-stream-alerts/internal/logging"
)

func TestConfigureLoggingCreatesFile(t *testing.T) {
	t.Cleanup(func() { logging.SetDefaultWriter(os.Stdout) })

	dir := t.TempDir()
	path := filepath.Join(dir, "alert.log")

	file, err := configureLogging(path)
	if err != nil {
		t.Fatalf("configure logging: %v", err)
	}
	file.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected new log file to be empty, got %d", info.Size())
	}
}

func TestRotateExistingLogArchivesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alertserver.log")
	if err := os.WriteFile(path, []byte("existing log"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	started := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)
	if err := rotateExistingLog(path, started); err != nil {
		t.Fatalf("rotate existing log: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected original log to be moved, stat err=%v", err)
	}

	archiveDir := filepath.Join(dir, "logs")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one rotated log file, found %d", len(entries))
	}
	expectedPrefix := "alertserver-" + started.Format("2006-01-02_15-04-05")
	if !strings.HasPrefix(entries[0].Name(), expectedPrefix) {
		t.Fatalf("expected archived log to start with %s, got %s", expectedPrefix, entries[0].Name())
	}
}
