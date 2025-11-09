// file name — /internal/youtube/subscribe.go
package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHubURL      = "https://pubsubhubbub.appspot.com/subscribe"
	defaultCallbackURL = "https://sharpen.live/alert"
	defaultLease       = 864000
)

// YouTubeRequest models the fields required by YouTube's WebSub subscription flow.
type YouTubeRequest struct {
	Topic        string
	Calback      string
	Mode         string
	Verify       string
	VerifyToken  string
	Secret       string
	LeaseSeconds int
}

// SubscribeOptions configures the HTTP handler that proxies subscribe requests to the hub.
type SubscribeOptions struct {
	HubURL string
	Client *http.Client
	Logger Logger
}

// NewSubscribeHandler returns an http.Handler that accepts POST requests and forwards them to YouTube's hub.
func NewSubscribeHandler(opts SubscribeOptions) http.Handler {
	hubURL := strings.TrimSpace(opts.HubURL)
	if hubURL == "" {
		hubURL = defaultHubURL
	}

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
		var subscribeReq YouTubeRequest
		if err := json.NewDecoder(r.Body).Decode(&subscribeReq); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		subscribeReq.Calback = defaultCallbackURL
		subscribeReq.Mode = "subscribe"
		if subscribeReq.LeaseSeconds <= 0 {
			subscribeReq.LeaseSeconds = defaultLease
		}

		requestHubURL := hubURL
		if override := strings.TrimSpace(r.URL.Query().Get("hub_url")); override != "" {
			requestHubURL = override
		}

		resp, body, err := SubscribeYouTube(r.Context(), client, requestHubURL, subscribeReq)
		if err != nil && opts.Logger != nil {
			opts.Logger.Printf("subscribe request hub response: %v", err)
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
	})
}

// YouTubeRequest is your provided shape; using as-is.
// Fields expected:
//
//	Topic, Calback (sic), Mode, Verify, VerifyToken, Secret, LeaseSeconds
//
// For WebSub subscribe, Mode should be "subscribe"; Verify is "sync" or "async".
func SubscribeYouTube(ctx context.Context, hc *http.Client, hubURL string, req YouTubeRequest) (*http.Response, []byte, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	if strings.TrimSpace(hubURL) == "" {
		return nil, nil, errors.New("hubURL is required")
	}
	if strings.TrimSpace(req.Topic) == "" {
		return nil, nil, errors.New("topic is required")
	}
	if strings.TrimSpace(req.Calback) == "" { // using field name exactly as provided
		return nil, nil, errors.New("calback (callback URL) is required")
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "subscribe"
	}
	if mode != "subscribe" {
		return nil, nil, fmt.Errorf("mode must be 'subscribe', got %q", mode)
	}
	verify := strings.TrimSpace(req.Verify)
	if verify == "" {
		verify = "async" // typical default for Google’s hub
	}
	if verify != "async" && verify != "sync" {
		return nil, nil, fmt.Errorf("verify must be 'sync' or 'async', got %q", verify)
	}

	// Build application/x-www-form-urlencoded body
	form := url.Values{}
	form.Set("hub.mode", mode)
	form.Set("hub.topic", req.Topic)
	form.Set("hub.callback", req.Calback)
	form.Set("hub.verify", verify)

	if req.VerifyToken != "" {
		form.Set("hub.verify_token", req.VerifyToken)
	}
	// Optional: request a lease duration; hub may ignore it.
	if req.LeaseSeconds > 0 {
		form.Set("hub.lease_seconds", fmt.Sprintf("%d", req.LeaseSeconds))
	}

	// Only include secret if callback is HTTPS (best practice)
	if req.Secret != "" {
		if u, err := url.Parse(req.Calback); err == nil && strings.EqualFold(u.Scheme, "https") {
			form.Set("hub.secret", req.Secret)
		}
	}

	// Build POST request
	hubURL = strings.TrimSpace(hubURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, hubURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", "sharpen-live-websub-client/1.0")

	// Send
	resp, err := hc.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("post to hub: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, fmt.Errorf("read hub response: %w", readErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, body, fmt.Errorf("hub returned non-2xx: %s", resp.Status)
	}
	return resp, body, nil
}
