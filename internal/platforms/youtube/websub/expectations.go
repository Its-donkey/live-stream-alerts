package websub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"time"
)

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

func GenerateVerifyToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fallback := fmt.Sprintf("%x", time.Now().UnixNano())
		return fallback
	}
	return hex.EncodeToString(b)
}

func RegisterExpectation(exp Expectation) {
	if exp.VerifyToken == "" {
		return
	}
	mu.Lock()
	expectations[exp.VerifyToken] = exp
	mu.Unlock()
}

func LookupExpectation(token string) (Expectation, bool) {
	mu.Lock()
	exp, ok := expectations[token]
	mu.Unlock()
	return exp, ok
}

func ConsumeExpectation(token string) (Expectation, bool) {
	mu.Lock()
	exp, ok := expectations[token]
	if ok {
		delete(expectations, token)
	}
	mu.Unlock()
	return exp, ok
}

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
