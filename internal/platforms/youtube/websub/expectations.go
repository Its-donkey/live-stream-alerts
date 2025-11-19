package websub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"time"
)

// Expectation captures the details of a pending hub verification callback.
type Expectation struct {
	Mode         string
	Topic        string
	VerifyToken  string
	LeaseSeconds int
	Secret       string
	ChannelID    string
	Alias        string
	HubStatus    string
	HubBody      string
}

var (
	expectations = make(map[string]Expectation)
	mu           sync.Mutex
)

// GenerateVerifyToken returns a random token used to correlate hub callbacks.
func GenerateVerifyToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fallback := fmt.Sprintf("%x", time.Now().UnixNano())
		return fallback
	}
	return hex.EncodeToString(b)
}

// RegisterExpectation stores the supplied expectation so callbacks can look it up.
func RegisterExpectation(exp Expectation) {
	if exp.VerifyToken == "" {
		return
	}
	mu.Lock()
	expectations[exp.VerifyToken] = exp
	mu.Unlock()
}

// LookupExpectation returns the expectation for the provided token without removing it.
func LookupExpectation(token string) (Expectation, bool) {
	mu.Lock()
	exp, ok := expectations[token]
	mu.Unlock()
	return exp, ok
}

// ConsumeExpectation returns and deletes the expectation associated with the token.
func ConsumeExpectation(token string) (Expectation, bool) {
	mu.Lock()
	exp, ok := expectations[token]
	if ok {
		delete(expectations, token)
	}
	mu.Unlock()
	return exp, ok
}

// CancelExpectation discards the expectation for the provided token.
func CancelExpectation(token string) {
	mu.Lock()
	delete(expectations, token)
	mu.Unlock()
}

// RecordSubscriptionResult stores data about the hub response so callers can log later.
func RecordSubscriptionResult(token, alias, topic, status, body string) {
	if token == "" {
		return
	}
	mu.Lock()
	exp, ok := expectations[token]
	if ok {
		if alias != "" {
			exp.Alias = alias
		}
		if topic != "" {
			exp.Topic = topic
		}
		exp.HubStatus = status
		exp.HubBody = body
		expectations[token] = exp
	}
	mu.Unlock()
}

// ExtractChannelID parses the channel ID from a YouTube topic URL.
func ExtractChannelID(topic string) string {
	if topic == "" {
		return ""
	}
	u, err := url.Parse(topic)
	if err != nil {
		return ""
	}
	return u.Query().Get("channel_id")
}
