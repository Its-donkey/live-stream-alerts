package adminhttp

import (
	"encoding/json"
	"net/http"
	"time"

	adminauth "live-stream-alerts/internal/admin/auth"
)

type LoginHandler struct {
	manager *adminauth.Manager
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

func NewLoginHandler(manager *adminauth.Manager) http.Handler {
	return LoginHandler{manager: manager}
}

func (h LoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.manager == nil {
		http.Error(w, "admin auth disabled", http.StatusServiceUnavailable)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	token, err := h.manager.Login(req.Email, req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	resp := loginResponse{Token: token.Value, ExpiresAt: token.ExpiresAt.Format(time.RFC3339)}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
