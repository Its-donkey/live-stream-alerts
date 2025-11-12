package handlers

import (
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

// HandleVerification handles YouTube PubSubHubbub GET verification requests.
// It returns true when the request has been handled (regardless of success).
func YouTubeSubscriptionConfirmation(w http.ResponseWriter, r *http.Request, logger logging.Logger) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch r.URL.Path {
	case "/alert", "/alerts":
	default:
		return false
	}

	query := r.URL.Query()
	challenge := query.Get("hub.challenge")
	if challenge == "" {
		http.Error(w, "missing hub.challenge", http.StatusBadRequest)
		return true
	}
	verifyToken := strings.TrimSpace(query.Get("hub.verify_token"))
	if verifyToken == "" {
		http.Error(w, "missing hub.verify_token", http.StatusBadRequest)
		return true
	}

	exp, ok := websub.LookupExpectation(verifyToken)
	if !ok {
		http.Error(w, "unknown verification token", http.StatusBadRequest)
		return true
	}

	topic := strings.TrimSpace(query.Get("hub.topic"))
	if exp.Topic != "" && topic != exp.Topic {
		http.Error(w, "hub.topic mismatch", http.StatusBadRequest)
		return true
	}

	leaseParam := strings.TrimSpace(query.Get("hub.lease_seconds"))
	leaseValue := 0
	if leaseParam != "" {
		parsedLease, err := strconv.Atoi(leaseParam)
		if err != nil {
			http.Error(w, "invalid hub.lease_seconds", http.StatusBadRequest)
			return true
		}
		leaseValue = parsedLease
	}
	if exp.LeaseSeconds > 0 && leaseValue != exp.LeaseSeconds {
		http.Error(w, "hub.lease_seconds mismatch", http.StatusBadRequest)
		return true
	}

	mode := strings.TrimSpace(query.Get("hub.mode"))
	if exp.Mode != "" && !strings.EqualFold(mode, exp.Mode) {
		http.Error(w, "hub.mode mismatch", http.StatusBadRequest)
		return true
	}

	if logger != nil {
		logger.Printf(
			"Responding to hub challenge: mode=%s topic=%s lease=%s token=%s body=%q",
			query.Get("hub.mode"),
			query.Get("hub.topic"),
			query.Get("hub.lease_seconds"),
			query.Get("hub.verify_token"),
			challenge,
		)
		if dump, err := httputil.DumpRequest(r, true); err == nil {
			logger.Printf("Raw verification request:\n%s", dump)
		} else {
			logger.Printf("Failed to dump verification request: %v", err)
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(challenge)))

	if logger != nil {
		var responseDump strings.Builder
		responseDump.WriteString("HTTP/1.1 200 OK\r\n")
		for name, values := range w.Header() {
			for _, value := range values {
				responseDump.WriteString(name)
				responseDump.WriteString(": ")
				responseDump.WriteString(value)
				responseDump.WriteString("\r\n")
			}
		}
		responseDump.WriteString("\r\n")
		responseDump.WriteString(challenge)
		logger.Printf("Planned hub response:\n%s", responseDump.String())
	}

	websub.ConsumeExpectation(verifyToken)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)

	return true
}
