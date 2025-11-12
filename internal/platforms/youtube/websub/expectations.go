package websub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Expectation struct {
	Mode         string
	Topic        string
	VerifyToken  string
	LeaseSeconds int
	Secret       string
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
