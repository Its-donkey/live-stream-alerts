// file name: internal/platforms/youtube/handlers/websub_alerts.go
package handlers

import (
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	youtubesub "live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
)

// SubscriptionConfirmationOptions configures how hub verification requests are handled.
type SubscriptionConfirmationOptions struct {
	Logger         logging.Logger
	StreamersStore *streamers.Store
}

type hubRequest struct {
	Challenge     string
	VerifyToken   string
	Topic         string
	Mode          string
	LeaseProvided bool
	LeaseValue    int
}

func (req hubRequest) IsUnsubscribe() bool {
	return strings.EqualFold(req.Mode, "unsubscribe")
}

// HandleSubscriptionConfirmation processes YouTube PubSubHubbub GET verification requests.
// It returns true when the request has been handled (regardless of success).
func HandleSubscriptionConfirmation(w http.ResponseWriter, r *http.Request, opts SubscriptionConfirmationOptions) bool {
	logger := opts.Logger
	if !isAlertsVerificationRequest(r) {
		return false
	}

	query := r.URL.Query()
	req, baseValidation := parseHubRequest(query)
	if !baseValidation.IsValid {
		http.Error(w, baseValidation.Error, http.StatusBadRequest)
		return true
	}

	exp, ok := websub.LookupExpectation(req.VerifyToken)
	if !ok {
		http.Error(w, "unknown verification token", http.StatusBadRequest)
		return true
	}

	expectationValidation := validateAgainstExpectation(req, exp)
	if !expectationValidation.IsValid {
		http.Error(w, expectationValidation.Error, http.StatusBadRequest)
		return true
	}

	logHubRequest(logger, r, query, req)

	prepareHubResponse(w, req.Challenge)
	logPlannedResponse(logger, w, req.Challenge)

	verifiedAt := time.Now().UTC()
	channelID := updateLeaseIfNeeded(req, exp, opts.StreamersStore, verifiedAt, logger)

	finalExp := finalizeExpectation(req.VerifyToken, exp)

	writeChallengeResponse(w, req.Challenge)

	logSubscriptionResult(logger, finalExp, exp, channelID, req.Topic, req.IsUnsubscribe())

	return true
}

func isAlertsVerificationRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/alerts"
}

func parseHubRequest(query url.Values) (hubRequest, ValidationResult) {
	req := hubRequest{
		Challenge:   query.Get("hub.challenge"),
		VerifyToken: strings.TrimSpace(query.Get("hub.verify_token")),
		Topic:       strings.TrimSpace(query.Get("hub.topic")),
		Mode:        strings.TrimSpace(query.Get("hub.mode")),
	}

	if req.Challenge == "" {
		return req, ValidationResult{IsValid: false, Error: "missing hub.challenge"}
	}
	if req.VerifyToken == "" {
		return req, ValidationResult{IsValid: false, Error: "missing hub.verify_token"}
	}

	leaseParam := strings.TrimSpace(query.Get("hub.lease_seconds"))
	if leaseParam != "" {
		parsedLease, err := strconv.Atoi(leaseParam)
		if err != nil {
			return req, ValidationResult{IsValid: false, Error: "invalid hub.lease_seconds"}
		}
		req.LeaseProvided = true
		req.LeaseValue = parsedLease
	}

	return req, ValidationResult{IsValid: true}
}

func validateAgainstExpectation(req hubRequest, exp websub.Expectation) ValidationResult {
	topic := req.Topic
	if exp.Topic != "" && topic != exp.Topic {
		return ValidationResult{IsValid: false, Error: "hub.topic mismatch"}
	}

	expIsUnsubscribe := strings.EqualFold(exp.Mode, "unsubscribe")
	leaseProvided := req.LeaseProvided
	if leaseProvided && exp.LeaseSeconds > 0 && req.LeaseValue != exp.LeaseSeconds && !expIsUnsubscribe {
		return ValidationResult{IsValid: false, Error: "hub.lease_seconds mismatch"}
	}

	if exp.Mode != "" && !strings.EqualFold(req.Mode, exp.Mode) {
		return ValidationResult{IsValid: false, Error: "hub.mode mismatch"}
	}

	return ValidationResult{IsValid: true}
}

func logHubRequest(logger logging.Logger, r *http.Request, query url.Values, req hubRequest) {
	if logger == nil {
		return
	}

	logger.Printf(
		"Responding to hub challenge: mode=%s topic=%s lease=%s token=%s body=%q",
		req.Mode,
		query.Get("hub.topic"),
		query.Get("hub.lease_seconds"),
		query.Get("hub.verify_token"),
		req.Challenge,
	)
	if dump, err := httputil.DumpRequest(r, true); err == nil {
		logger.Printf("Raw verification request:\n%s", dump)
	} else {
		logger.Printf("Failed to dump verification request: %v", err)
	}
}

func prepareHubResponse(w http.ResponseWriter, challenge string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(challenge)))
}

func logPlannedResponse(logger logging.Logger, w http.ResponseWriter, challenge string) {
	if logger == nil {
		return
	}

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

func updateLeaseIfNeeded(req hubRequest, exp websub.Expectation, store *streamers.Store, verifiedAt time.Time, logger logging.Logger) string {
	channelID := exp.ChannelID
	if channelID == "" {
		channelID = websub.ExtractChannelID(req.Topic)
	}

	if channelID != "" && !req.IsUnsubscribe() && req.LeaseProvided {
		if err := youtubesub.RecordLease(store, channelID, verifiedAt); err != nil && logger != nil {
			logger.Printf("failed to record hub lease for %s: %v", channelID, err)
		}
	}

	return channelID
}

func finalizeExpectation(verifyToken string, exp websub.Expectation) websub.Expectation {
	finalExp := exp
	if consumed, ok := websub.ConsumeExpectation(verifyToken); ok {
		finalExp = consumed
	}
	return finalExp
}

func writeChallengeResponse(w http.ResponseWriter, challenge string) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, challenge)
}

func logSubscriptionResult(logger logging.Logger, finalExp, originalExp websub.Expectation, channelID, topic string, isUnsubscribe bool) {
	if logger == nil {
		return
	}

	if finalExp.HubStatus != "" {
		logger.Printf("YouTube hub response status: %s, body: %s", finalExp.HubStatus, finalExp.HubBody)
	}
	alias := strings.TrimSpace(finalExp.Alias)
	if alias == "" {
		alias = strings.TrimSpace(originalExp.Alias)
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
		displayTopic = originalExp.Topic
	}

	if isUnsubscribe {
		logger.Printf("YouTube alerts unsubscribed for %s (%s)", alias, displayTopic)
	} else {
		logger.Printf("YouTube alerts subscribed for %s (%s)", alias, displayTopic)
	}
}
