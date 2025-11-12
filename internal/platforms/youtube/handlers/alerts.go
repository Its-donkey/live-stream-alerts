package handlers

import (
	"io"
	"net/http"
	"strconv"

	"live-stream-alerts/internal/logging"
)

// HandleVerification handles YouTube PubSubHubbub GET verification requests.
// It returns true when the request has been handled (regardless of success).
func SubscriptionConfirmation(w http.ResponseWriter, r *http.Request, logger logging.Logger) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch r.URL.Path {
	case "/alerts", "/alert":
	default:
		return false
	}

	query := r.URL.Query()
	challenge := query.Get("hub.challenge")
	if challenge == "" {
		http.Error(w, "missing hub.challenge", http.StatusBadRequest)
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
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(challenge)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)

	if logger != nil {
		logger.Printf("Hub challenge reply sent with status=200 body=%q", challenge)
	}
	return true
}
