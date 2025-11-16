package handlers

import (
	"encoding/json"
	"io"
	"net/http"
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
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()
		var req subscriptions.YouTubeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		req.Mode = mode
		if req.LeaseSeconds <= 0 {
			req.LeaseSeconds = config.YT.LeaseSeconds
		}

		resp, body, finalReq, err := subscriptions.SubscribeYouTube(r.Context(), client, opts.Logger, req)
		if err != nil && opts.Logger != nil {
			opts.Logger.Printf("%s hub response: %v", logLabel, err)
		}
		if resp == nil {
			http.Error(w, "hub request failed", http.StatusBadGateway)
			return
		}

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

		websub.RecordSubscriptionResult(finalReq.VerifyToken, "", req.Topic, resp.Status, string(body))
	})
}
