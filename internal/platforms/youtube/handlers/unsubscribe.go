package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

// UnsubscribeHandlerOptions configures the unsubscribe handler.
type UnsubscribeHandlerOptions struct {
	HubURL string
	Client *http.Client
	Logger logging.Logger
}

// NewUnsubscribeHandler returns an http.Handler that issues unsubscribe requests to YouTube's hub.
func NewUnsubscribeHandler(opts UnsubscribeHandlerOptions) http.Handler {
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
		var unsubscribeReq subscriptions.YouTubeRequest
		if err := json.NewDecoder(r.Body).Decode(&unsubscribeReq); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		subscriptions.NormaliseUnsubscribeRequest(&unsubscribeReq)
		requestHubURL := subscriptions.DefaultHubURL
		if opts.HubURL != "" {
			requestHubURL = opts.HubURL
		}

		resp, body, finalReq, err := subscriptions.SubscribeYouTube(r.Context(), client, requestHubURL, unsubscribeReq)
		if err != nil && opts.Logger != nil {
			opts.Logger.Printf("unsubscribe request hub response: %v", err)
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

		websub.RecordSubscriptionResult(finalReq.VerifyToken, "", unsubscribeReq.Topic, resp.Status, string(body))
	})
}
