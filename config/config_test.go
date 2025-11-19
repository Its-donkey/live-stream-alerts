package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{},"youtube":{},"admin":{}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.Addr != defaultAddr {
		t.Fatalf("expected default addr %s, got %s", defaultAddr, cfg.Server.Addr)
	}
	if cfg.Server.Port != defaultPort {
		t.Fatalf("expected default port %s, got %s", defaultPort, cfg.Server.Port)
	}
	if cfg.Admin.TokenTTLSeconds != 86400 {
		t.Fatalf("expected default admin ttl, got %d", cfg.Admin.TokenTTLSeconds)
	}
}

func TestLoadHonoursOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{
		"server": {"addr":"0.0.0.0","port":":9999"},
		"youtube": {"hub_url":"https://hub","callback_url":"https://callback","lease_seconds":123},
		"admin": {"email":"admin@example.com","password":"secret","token_ttl_seconds":10}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.Addr != "0.0.0.0" || cfg.Server.Port != ":9999" {
		t.Fatalf("server overrides not applied: %+v", cfg.Server)
	}
	if cfg.YouTube.HubURL != "https://hub" || cfg.YouTube.LeaseSeconds != 123 {
		t.Fatalf("youtube overrides not applied: %+v", cfg.YouTube)
	}
	if cfg.Admin.Email != "admin@example.com" || cfg.Admin.TokenTTLSeconds != 10 {
		t.Fatalf("admin overrides not applied: %+v", cfg.Admin)
	}
}

func TestLoadErrorsForMissingFile(t *testing.T) {
	if _, err := Load("missing.json"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}
