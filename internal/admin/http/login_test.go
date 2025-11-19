package adminhttp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminauth "live-stream-alerts/internal/admin/auth"
	adminhttp "live-stream-alerts/internal/admin/http"
	adminservice "live-stream-alerts/internal/admin/service"
)

func TestLoginHandlerSuccess(t *testing.T) {
	token := adminauth.Token{Value: "abc123", ExpiresAt: time.Now().Add(time.Hour)}
	service := &stubLoginService{token: token}
	handler := adminhttp.NewLoginHandler(adminhttp.LoginHandlerOptions{Service: service})

	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if service.email != "admin@example.com" || service.password != "secret" {
		t.Fatalf("service received unexpected credentials: %+v", service)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] != token.Value {
		t.Fatalf("expected token %q, got %q", token.Value, resp["token"])
	}
}

func TestLoginHandlerHandlesErrors(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
	}{
		{"invalid credentials", adminservice.ErrInvalidCredentials, http.StatusUnauthorized},
		{"unauthorized service", adminservice.ErrUnauthorized, http.StatusServiceUnavailable},
		{"generic failure", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := adminhttp.NewLoginHandler(adminhttp.LoginHandlerOptions{
				Service: &stubLoginService{err: tc.err},
			})
			body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "secret"})
			req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			if rr.Code != tc.expectedStatus {
				t.Fatalf("expected %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}

type stubLoginService struct {
	email    string
	password string
	token    adminauth.Token
	err      error
}

func (s *stubLoginService) Login(email, password string) (adminauth.Token, error) {
	s.email = email
	s.password = password
	return s.token, s.err
}
