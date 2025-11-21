package auth

import (
	"testing"
	"time"
)

func TestManagerLoginAndValidate(t *testing.T) {
	mgr := NewManager(Config{Email: "admin@example.com", Password: "secret", TokenTTL: time.Minute})
	token, err := mgr.Login("admin@example.com", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if token.Value == "" {
		t.Fatalf("expected token value")
	}
	if !mgr.Validate(token.Value) {
		t.Fatalf("expected token to validate")
	}
}

func TestManagerRejectsBadCredentials(t *testing.T) {
	mgr := NewManager(Config{Email: "admin@example.com", Password: "secret"})
	if _, err := mgr.Login("admin@example.com", "wrong"); err == nil {
		t.Fatalf("expected error for wrong password")
	}
	if _, err := mgr.Login("", "secret"); err == nil {
		t.Fatalf("expected error for empty email")
	}
}

func TestManagerExpiresTokens(t *testing.T) {
	mgr := NewManager(Config{Email: "admin@example.com", Password: "secret", TokenTTL: time.Millisecond})
	token, err := mgr.Login("admin@example.com", "secret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if mgr.Validate(token.Value) {
		t.Fatalf("expected token to expire")
	}
}
