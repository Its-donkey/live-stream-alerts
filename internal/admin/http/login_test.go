package adminhttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminauth "live-stream-alerts/internal/admin/auth"
	adminhttp "live-stream-alerts/internal/admin/http"
)

func TestLoginHandlerSuccess(t *testing.T) {
	mgr := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret", TokenTTL: time.Minute})
	handler := adminhttp.NewLoginHandler(mgr)

	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Fatalf("expected token in response")
	}
}

func TestLoginHandlerRejectsInvalidCreds(t *testing.T) {
	mgr := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"})
	handler := adminhttp.NewLoginHandler(mgr)

	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "bad"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
