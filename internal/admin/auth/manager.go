package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// Config captures the credentials and TTL required to issue admin tokens.
type Config struct {
	Email    string
	Password string
	TokenTTL time.Duration
}

// Token represents a bearer token issued after a successful login.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

// Manager issues and validates admin bearer tokens.
type Manager struct {
	email    string
	password string
	tokenTTL time.Duration

	mu     sync.Mutex
	tokens map[string]time.Time
}

// ErrInvalidCredentials indicates that the provided email/password pair was rejected.
var ErrInvalidCredentials = errors.New("invalid credentials")

// NewManager returns a Manager initialised with the supplied config.
func NewManager(cfg Config) *Manager {
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{
		email:    strings.ToLower(strings.TrimSpace(cfg.Email)),
		password: cfg.Password,
		tokenTTL: ttl,
		tokens:   make(map[string]time.Time),
	}
}

// Login validates the provided credentials and returns a short-lived token.
func (m *Manager) Login(email, password string) (Token, error) {
	if m == nil {
		return Token{}, ErrInvalidCredentials
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return Token{}, ErrInvalidCredentials
	}
	if email != m.email || password != m.password {
		return Token{}, ErrInvalidCredentials
	}

	token := Token{
		Value:     generateToken(),
		ExpiresAt: time.Now().UTC().Add(m.tokenTTL),
	}

	m.mu.Lock()
	m.tokens[token.Value] = token.ExpiresAt
	m.mu.Unlock()

	return token, nil
}

// Validate checks whether the provided token exists and has not expired.
func (m *Manager) Validate(token string) bool {
	if m == nil {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	expiry, ok := m.tokens[token]
	if !ok {
		return false
	}
	if time.Now().UTC().After(expiry) {
		delete(m.tokens, token)
		return false
	}
	return true
}

func generateToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}
