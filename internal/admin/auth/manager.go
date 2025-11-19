package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Email    string
	Password string
	TokenTTL time.Duration
}

type Token struct {
	Value     string
	ExpiresAt time.Time
}

type Manager struct {
	email    string
	password string
	tokenTTL time.Duration

	mu     sync.Mutex
	tokens map[string]time.Time
}

var ErrInvalidCredentials = errors.New("invalid credentials")

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
