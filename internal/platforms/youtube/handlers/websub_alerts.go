// file name: internal/platforms/youtube/handlers/websub_alerts.go
package handlers

import (
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	youtubesub "live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

// SubscriptionConfirmationOptions configures how hub verification requests are handled.
type SubscriptionConfirmationOptions struct {
	Logger        logging.Logger
	StreamersPath string
}

// HandleSubscriptionConfirmation processes YouTube PubSubHubbub GET verification requests.
// It returns true when the request has been handled (regardless of success).
func HandleSubscriptionConfirmation(w http.ResponseWriter, r *http.Request, opts SubscriptionConfirmationOptions) bool {
	logger := opts.Logger
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
	leaseProvided := leaseParam != ""
	if leaseProvided {
		parsedLease, err := strconv.Atoi(leaseParam)
		if err != nil {
			http.Error(w, "invalid hub.lease_seconds", http.StatusBadRequest)
			return true
		}
		leaseValue = parsedLease
	}
	expIsUnsubscribe := strings.EqualFold(exp.Mode, "unsubscribe")
	if leaseProvided && exp.LeaseSeconds > 0 && leaseValue != exp.LeaseSeconds && !expIsUnsubscribe {
		http.Error(w, "hub.lease_seconds mismatch", http.StatusBadRequest)
		return true
	}

	mode := strings.TrimSpace(query.Get("hub.mode"))
	isUnsubscribe := strings.EqualFold(mode, "unsubscribe")
	if exp.Mode != "" && !strings.EqualFold(mode, exp.Mode) {
		http.Error(w, "hub.mode mismatch", http.StatusBadRequest)
		return true
	}

	if logger != nil {
		logger.Printf(
			"Responding to hub challenge: mode=%s topic=%s lease=%s token=%s body=%q",
			mode,
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

	verifiedAt := time.Now().UTC()
	channelID := exp.ChannelID
	if channelID == "" {
		channelID = websub.ExtractChannelID(topic)
	}
	if channelID != "" && !isUnsubscribe && leaseProvided {
		if err := youtubesub.RecordLease(opts.StreamersPath, channelID, verifiedAt); err != nil && logger != nil {
			logger.Printf("failed to record hub lease for %s: %v", channelID, err)
		}
	}

	finalExp := exp
	if consumed, ok := websub.ConsumeExpectation(verifyToken); ok {
		finalExp = consumed
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)

	if logger != nil {
		if finalExp.HubStatus != "" {
			logger.Printf("YouTube hub response status: %s, body: %s", finalExp.HubStatus, finalExp.HubBody)
		}
		alias := strings.TrimSpace(finalExp.Alias)
		if alias == "" {
			alias = strings.TrimSpace(exp.Alias)
		}
		if alias == "" {
			alias = channelID
		}
		if alias == "" {
			alias = "channel"
		}
		displayTopic := topic
		if displayTopic == "" {
			displayTopic = finalExp.Topic
		}
		if displayTopic == "" {
			displayTopic = exp.Topic
		}
		if isUnsubscribe {
			logger.Printf("YouTube alerts unsubscribed for %s (%s)", alias, displayTopic)
		} else {
			logger.Printf("YouTube alerts subscribed for %s (%s)", alias, displayTopic)
		}
	}

	return true
}
