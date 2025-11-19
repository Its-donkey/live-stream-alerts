// file name — /client.go
package subscriptions

import (
	"bytes"
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

// ErrValidation signals that the request payload is missing required fields.
var ErrValidation = errors.New("validation error")

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

// SubscribeYouTube executes a WebSub subscription call against the provided hub URL.
// For WebSub subscribe, Mode should be "subscribe"; for unsubscribe, "unsubscribe".
// Verify is "sync" or "async".
func SubscribeYouTube(
	ctx context.Context,
	hc *http.Client,
	logger logging.Logger,
	req YouTubeRequest,
) (*http.Response, []byte, YouTubeRequest, error) {
	// Ensure we have a logger.
	if logger == nil {
		logger = logging.New()
	}

	// Centralised default HTTP client.
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}

	// Resolve hub URL.
	hubURL := strings.TrimSpace(req.HubURL)
	if hubURL == "" {
		return nil, nil, req, errors.New("hubURL is required")
	}

	if strings.TrimSpace(req.Topic) == "" {
		return nil, nil, req, fmt.Errorf("%w: topic is required", ErrValidation)
	}

	// Resolve callback.
	callback := strings.TrimSpace(req.Callback)
	if callback == "" {
		return nil, nil, req, errors.New("callback is required")
	}

	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		return nil, nil, req, fmt.Errorf("%w: subscribe or unsubscribe must be set as mode", ErrValidation)
	}

	// Resolve verify.
	verify := strings.TrimSpace(req.Verify)
	if verify == "" {
		return nil, nil, req, errors.New("verify mode must be set")
	}

	verifyToken := strings.TrimSpace(websub.GenerateVerifyToken())
	if verifyToken == "" {
		return nil, nil, req, errors.New("verify token generation failed")
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

	// Build application/x-www-form-urlencoded body.
	form := url.Values{}
	form.Set("hub.mode", mode)
	form.Set("hub.topic", req.Topic)
	form.Set("hub.callback", callback)
	form.Set("hub.verify", verify)
	form.Set("hub.verify_token", req.VerifyToken)
	if req.LeaseSeconds > 0 {
		form.Set("hub.lease_seconds", strconv.Itoa(req.LeaseSeconds))
	}

	// Only include secret if callback is HTTPS (best practice).
	// Uses the resolved callback, so it works whether it came from req or config.
	if req.Secret != "" {
		if u, err := url.Parse(callback); err == nil && strings.EqualFold(u.Scheme, "https") {
			form.Set("hub.secret", req.Secret)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, hubURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, req, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", "live-stream-alerts-client/1.0")

	if dump, err := httputil.DumpRequestOut(httpReq, true); err == nil {
		logger.Printf("Outbound WebSub request:\n%s", dump)
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
	resp.Body = io.NopCloser(bytes.NewReader(body))

	if dump, err := httputil.DumpResponse(resp, true); err == nil {
		logger.Printf("Inbound WebSub response:\n%s", dump)
	} else {
		logger.Printf("Failed to dump WebSub response: %v", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, body, req, fmt.Errorf("hub returned non-2xx: %s", resp.Status)
	}

	subscriptionAccepted = true
	return resp, body, req, nil
}
