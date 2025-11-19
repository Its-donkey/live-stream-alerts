package service

import (
	"errors"
	"net/http"
	"strings"

	adminauth "live-stream-alerts/internal/admin/auth"
)

// AuthService provides helpers for validating admin credentials and tokens.
type AuthService struct {
	Manager *adminauth.Manager
}

// AuthorizeRequest validates the Authorization header on the provided request.
// It returns ErrUnauthorized when the token is missing or invalid.
func (s AuthService) AuthorizeRequest(r *http.Request) error {
	if s.Manager == nil {
		return ErrUnauthorized
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ErrUnauthorized
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ErrUnauthorized
	}
	if token := strings.TrimSpace(parts[1]); token != "" && s.Manager.Validate(token) {
		return nil
	}
	return ErrUnauthorized
}

// Login verifies the provided credentials and returns a scoped token.
func (s AuthService) Login(email, password string) (adminauth.Token, error) {
	if s.Manager == nil {
		return adminauth.Token{}, ErrUnauthorized
	}
	token, err := s.Manager.Login(email, password)
	if err != nil {
		return adminauth.Token{}, ErrInvalidCredentials
	}
	return token, nil
}

var (
	// ErrUnauthorized indicates the caller lacks a valid admin token.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidCredentials signals bad login credentials.
	ErrInvalidCredentials = errors.New("invalid credentials")
)
