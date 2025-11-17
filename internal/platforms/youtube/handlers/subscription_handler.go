package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/config"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

// SubscriptionHandlerOptions configures handlers that talk to the YouTube hub.
type SubscriptionHandlerOptions struct {
	Client *http.Client
	Logger logging.Logger
}

// SubscribeHandlerOptions configures the subscribe handler.
type SubscribeHandlerOptions = SubscriptionHandlerOptions

// UnsubscribeHandlerOptions configures the unsubscribe handler.
type UnsubscribeHandlerOptions = SubscriptionHandlerOptions

// NewSubscribeHandler returns an http.Handler that accepts POST requests and forwards them to YouTube's hub.
func NewSubscribeHandler(opts SubscribeHandlerOptions) http.Handler {
	return newSubscriptionHandler("subscribe", "subscribe request", opts)
}

// NewUnsubscribeHandler returns an http.Handler that issues unsubscribe requests to YouTube's hub.
func NewUnsubscribeHandler(opts UnsubscribeHandlerOptions) http.Handler {
	return newSubscriptionHandler("unsubscribe", "unsubscribe request", opts)
}

func newSubscriptionHandler(mode, logLabel string, opts SubscriptionHandlerOptions) http.Handler {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPostRequest(r) {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		defer r.Body.Close()

		req, decodeResult := decodeSubscriptionRequest(r)
		if !decodeResult.IsValid {
			http.Error(w, decodeResult.Error, http.StatusBadRequest)
			return
		}

		applySubscriptionDefaults(&req, mode)

		resp, body, finalReq, err := subscriptions.SubscribeYouTube(r.Context(), client, opts.Logger, req)
		if handled := handleSubscriptionError(w, resp, err, logLabel, opts.Logger); handled {
			return
		}
		if resp == nil {
			http.Error(w, "hub request failed", http.StatusBadGateway)
			return
		}

		writeSubscriptionResponse(w, resp, body)
		recordSubscriptionResult(finalReq, req, resp, body)
	})
}

func decodeSubscriptionRequest(r *http.Request) (subscriptions.YouTubeRequest, ValidationResult) {
	var req subscriptions.YouTubeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, ValidationResult{IsValid: false, Error: "invalid JSON body"}
	}

	req.Topic = strings.TrimSpace(req.Topic)
	if req.Topic == "" {
		return req, ValidationResult{IsValid: false, Error: "topic is required"}
	}

	return req, ValidationResult{IsValid: true}
}

func applySubscriptionDefaults(req *subscriptions.YouTubeRequest, mode string) {
	req.Mode = mode
	if strings.EqualFold(mode, "subscribe") && req.LeaseSeconds <= 0 {
		req.LeaseSeconds = config.YT.LeaseSeconds
	}
}

func handleSubscriptionError(w http.ResponseWriter, resp *http.Response, err error, logLabel string, logger logging.Logger) bool {
	if err == nil {
		return false
	}
	if logger != nil {
		logger.Printf("%s hub response: %v", logLabel, err)
	}
	if resp == nil {
		status := http.StatusBadGateway
		if errors.Is(err, subscriptions.ErrValidation) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return true
	}
	return false
}

func writeSubscriptionResponse(w http.ResponseWriter, resp *http.Response, body []byte) {
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(resp.StatusCode)
	if len(body) > 0 {
		_, _ = w.Write(body)
		return
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.WriteString(w, resp.Status)
	}
}

func recordSubscriptionResult(finalReq, originalReq subscriptions.YouTubeRequest, resp *http.Response, body []byte) {
	token := strings.TrimSpace(finalReq.VerifyToken)
	if token == "" {
		token = strings.TrimSpace(originalReq.VerifyToken)
	}
	if token == "" {
		return
	}

	status := ""
	if resp != nil {
		status = resp.Status
	}

	websub.RecordSubscriptionResult(token, "", originalReq.Topic, status, string(body))
}
