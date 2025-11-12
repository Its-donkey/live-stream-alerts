package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

const (
	DefaultHubURL      = "https://pubsubhubbub.appspot.com/subscribe"
	DefaultCallbackURL = "https://sharpen.live/alerts"
	DefaultLease       = 864000
	DefaultMode        = "subscribe"
)

// YouTubeRequest models the fields required by YouTube's WebSub subscription flow.
type YouTubeRequest struct {
	Topic        string
	Callback     string
	Mode         string
	Verify       string
	VerifyToken  string
	Secret       string
	LeaseSeconds int
	ChannelID    string
}

// NormaliseSubscribeRequest applies the enforced defaults required by the system.
func NormaliseSubscribeRequest(req *YouTubeRequest) {
	req.Callback = DefaultCallbackURL
	req.Mode = DefaultMode
	req.LeaseSeconds = DefaultLease
}

// YouTubeRequest is your provided shape; using as-is.
// Fields expected:
//
//	Topic, Callback (sic), Mode, Verify, VerifyToken, Secret, LeaseSeconds
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
	if strings.TrimSpace(req.Callback) == "" { // using field name exactly as provided
		return nil, nil, errors.New("callback url is required")
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

	if strings.TrimSpace(req.VerifyToken) == "" {
		req.VerifyToken = websub.GenerateVerifyToken()
	}

	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		channelID = websub.ExtractChannelID(req.Topic)
	}

	websub.RegisterExpectation(websub.Expectation{
		Mode:         mode,
		Topic:        req.Topic,
		VerifyToken:  req.VerifyToken,
		LeaseSeconds: req.LeaseSeconds,
		Secret:       req.Secret,
		ChannelID:    channelID,
	})
	registeredToken := req.VerifyToken
	subscriptionAccepted := false
	defer func() {
		if !subscriptionAccepted {
			websub.CancelExpectation(registeredToken)
		}
	}()

	// Build application/x-www-form-urlencoded body
	form := url.Values{}
	form.Set("hub.mode", mode)
	form.Set("hub.topic", req.Topic)
	form.Set("hub.callback", req.Callback)
	form.Set("hub.verify", verify)
	form.Set("hub.verify_token", req.VerifyToken)
	// Optional: request a lease duration; hub may ignore it.
	if req.LeaseSeconds > 0 {
		form.Set("hub.lease_seconds", fmt.Sprintf("%d", req.LeaseSeconds))
	}

	// Only include secret if callback is HTTPS (best practice)
	if req.Secret != "" {
		if u, err := url.Parse(req.Callback); err == nil && strings.EqualFold(u.Scheme, "https") {
			form.Set("hub.secret", req.Secret)
		}
	}

	// fmt.Println(form.Encode())
	logging.New().Printf("Submitting YouTube WebSub subscription: hub=%s topic=%s callback=%s mode=%s verify=%s lease=%d",
		hubURL, req.Topic, req.Callback, mode, verify, req.LeaseSeconds)

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
	subscriptionAccepted = true
	return resp, body, nil
}
