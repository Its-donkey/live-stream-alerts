package service

import (
	"net/http/httptest"
	"testing"

	adminauth "live-stream-alerts/internal/admin/auth"
)

func TestAuthServiceAuthorizeRequest(t *testing.T) {
	mgr := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"})
	token, err := mgr.Login("admin@example.com", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	service := AuthService{Manager: mgr}

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token.Value)
	if err := service.AuthorizeRequest(req); err != nil {
		t.Fatalf("expected authorization success, got %v", err)
	}

	req.Header.Set("Authorization", "Bearer bad-token")
	if err := service.AuthorizeRequest(req); err == nil {
		t.Fatalf("expected authorization failure")
	}
}

func TestAuthServiceLogin(t *testing.T) {
	mgr := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"})
	service := AuthService{Manager: mgr}

	if _, err := service.Login("admin@example.com", "secret"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := service.Login("admin@example.com", "wrong"); err == nil {
		t.Fatalf("expected invalid credentials")
	}
}
