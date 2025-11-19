package adminhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	adminauth "live-stream-alerts/internal/admin/auth"
	adminservice "live-stream-alerts/internal/admin/service"
)

// LoginHandlerOptions configures the admin login handler.
type LoginHandlerOptions struct {
	Service loginService
	Manager *adminauth.Manager
}

type loginService interface {
	Login(email, password string) (adminauth.Token, error)
}

// LoginHandler exposes the admin login endpoint.
type LoginHandler struct {
	service loginService
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

// NewLoginHandler constructs the admin login handler.
func NewLoginHandler(opts LoginHandlerOptions) http.Handler {
	svc := opts.Service
	if svc == nil && opts.Manager != nil {
		svc = adminservice.AuthService{Manager: opts.Manager}
	}
	return LoginHandler{service: svc}
}

func (h LoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		http.Error(w, "admin auth disabled", http.StatusServiceUnavailable)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	token, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, adminservice.ErrInvalidCredentials):
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
		case errors.Is(err, adminservice.ErrUnauthorized):
			http.Error(w, "admin auth disabled", http.StatusServiceUnavailable)
		default:
			http.Error(w, "failed to authenticate", http.StatusInternalServerError)
		}
		return
	}

	resp := loginResponse{Token: token.Value, ExpiresAt: token.ExpiresAt.Format(time.RFC3339)}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
