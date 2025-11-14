package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

const (
	DefaultHubURL      = "https://pubsubhubbub.appspot.com/subscribe"
	DefaultCallbackURL = "https://sharpen.live/alerts"
	DefaultLease       = 864000 // 10 days in seconds (maximum for YouTube)
	DefaultMode        = "subscribe"
	DefaultVerify      = "async"
)

// YouTubeRequest models the fields required by YouTube's WebSub subscription flow.
type YouTubeRequest struct {
	HubURL       string
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
	if strings.TrimSpace(req.HubURL) == "" {
		req.HubURL = DefaultHubURL
	}
	if strings.TrimSpace(req.Callback) == "" {
		req.Callback = DefaultCallbackURL
	}
	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = DefaultMode
	}
	if strings.TrimSpace(req.Verify) == "" {
		req.Verify = DefaultVerify
	}
	if req.LeaseSeconds == 0 {
		req.LeaseSeconds = DefaultLease
	}
}

// SubscribeYouTube executes a WebSub subscription call against the provided hub URL.
// Fields expected:
//   - Topic, Callback, Mode, Verify, VerifyToken, Secret, LeaseSeconds
//
// For WebSub subscribe, Mode should be "subscribe"; Verify is "sync" or "async".
func SubscribeYouTube(ctx context.Context, hc *http.Client, hubURL string, req YouTubeRequest) (*http.Response, []byte, YouTubeRequest, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	if strings.TrimSpace(hubURL) == "" {
		return nil, nil, req, errors.New("hubURL is required")
	}
	if strings.TrimSpace(req.Topic) == "" {
		return nil, nil, req, errors.New("topic is required")
	}
	callback := strings.TrimSpace(req.Callback)
	if callback == "" {
		callback = DefaultCallbackURL
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = DefaultMode
	}
	verify := strings.TrimSpace(req.Verify)
	if verify == "" {
		verify = DefaultVerify
	}
	verifyToken := strings.TrimSpace(req.VerifyToken)
	if verifyToken == "" {
		verifyToken = websub.GenerateVerifyToken()
	}
	req.VerifyToken = verifyToken

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
	form.Set("hub.callback", callback)
	form.Set("hub.verify", verify)
	form.Set("hub.verify_token", req.VerifyToken)
	if req.LeaseSeconds > 0 {
		form.Set("hub.lease_seconds", strconv.Itoa(req.LeaseSeconds))
	}

	// Only include secret if callback is HTTPS (best practice)
	if req.Secret != "" {
		if u, err := url.Parse(req.Callback); err == nil && strings.EqualFold(u.Scheme, "https") {
			form.Set("hub.secret", req.Secret)
		}
	}

	hubURL = strings.TrimSpace(hubURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, hubURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, req, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", "sharpen-live-websub-client/1.0")

	if dump, err := httputil.DumpRequestOut(httpReq, true); err == nil {
		logging.New().Printf("Outbound WebSub request:\n%s", dump)
	}

	resp, err := hc.Do(httpReq)
	if err != nil {
		return nil, nil, req, fmt.Errorf("post to hub: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, req, fmt.Errorf("read hub response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, body, req, fmt.Errorf("hub returned non-2xx: %s", resp.Status)
	}
	subscriptionAccepted = true
	return resp, body, req, nil
}
