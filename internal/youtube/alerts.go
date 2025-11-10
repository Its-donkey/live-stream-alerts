package youtube

import (
	"io"
	"net/http"
)

// Logger captures the subset of log.Logger we need so callers can pass custom loggers.
type Logger interface {
	Printf(format string, v ...any)
}

// HandleVerification handles YouTube PubSubHubbub GET verification requests.
// It returns true when the request has been handled (regardless of success).
func HandleAlertsVerification(w http.ResponseWriter, r *http.Request, logger Logger) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/alerts" {
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
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)
	return true
}
