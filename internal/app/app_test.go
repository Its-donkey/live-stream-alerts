package app

import (
	"testing"
	"time"

	"live-stream-alerts/config"
)

func TestOptionsWithDefaults(t *testing.T) {
	opts := Options{}
	normalized := opts.withDefaults()

	if normalized.ConfigPath != defaultConfigPath {
		t.Fatalf("expected default config path, got %q", normalized.ConfigPath)
	}
	if normalized.LogDir != defaultLogDir {
		t.Fatalf("expected default log dir, got %q", normalized.LogDir)
	}
	if normalized.LogFile != defaultLogFileName {
		t.Fatalf("expected default log file, got %q", normalized.LogFile)
	}
	if normalized.ReadTimeout != defaultReadTimeout {
		t.Fatalf("expected default read timeout, got %s", normalized.ReadTimeout)
	}
}

func TestOptionsWithDefaultsRespectOverrides(t *testing.T) {
	opts := Options{
		ConfigPath:  "custom.json",
		LogDir:      "logs",
		LogFile:     "app.log",
		ReadTimeout: 5 * time.Second,
	}
	normalized := opts.withDefaults()

	if normalized != opts {
		t.Fatalf("expected overrides to remain unchanged")
	}
}

func TestBuildAdminManagerRequiresCredentials(t *testing.T) {
	if mgr := buildAdminManager(config.AdminConfig{}); mgr != nil {
		t.Fatalf("expected nil manager for missing credentials")
	}

	cfg := config.AdminConfig{Email: "admin@example.com", Password: "secret", TokenTTLSeconds: 60}
	mgr := buildAdminManager(cfg)
	if mgr == nil {
		t.Fatalf("expected manager for populated credentials")
	}
}
