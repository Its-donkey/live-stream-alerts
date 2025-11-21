package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

const defaultSubscriptionTimeout = 10 * time.Second

// SubscriptionResult describes the upstream hub response.
type SubscriptionResult struct {
	StatusCode  int
	ContentType string
	Body        []byte
	StatusText  string
}

// ProxyError wraps hub failures with the HTTP status code clients should see.
type ProxyError struct {
	Status int
	Err    error
}

// Error implements error.
func (e *ProxyError) Error() string {
	return e.Err.Error()
}

// Unwrap exposes the underlying error.
func (e *ProxyError) Unwrap() error {
	return e.Err
}

// SubscriptionProxy issues subscribe/unsubscribe requests on behalf of handlers.
type SubscriptionProxy struct {
	mode         string
	client       *http.Client
	logger       logging.Logger
	hubURL       string
	callbackURL  string
	verifyMode   string
	leaseSeconds int
	timeout      time.Duration
}

// SubscriptionProxyOptions configures the proxy defaults.
type SubscriptionProxyOptions struct {
	Client       *http.Client
	Logger       logging.Logger
	HubURL       string
	CallbackURL  string
	VerifyMode   string
	LeaseSeconds int
	Timeout      time.Duration
}

// NewSubscriptionProxy returns a proxy for the supplied mode (subscribe/unsubscribe).
func NewSubscriptionProxy(mode string, opts SubscriptionProxyOptions) *SubscriptionProxy {
	return &SubscriptionProxy{
		mode:         mode,
		client:       opts.Client,
		logger:       opts.Logger,
		hubURL:       strings.TrimSpace(opts.HubURL),
		callbackURL:  strings.TrimSpace(opts.CallbackURL),
		verifyMode:   strings.TrimSpace(opts.VerifyMode),
		leaseSeconds: opts.LeaseSeconds,
		timeout:      opts.Timeout,
	}
}

// Process forwards the request to the YouTube hub and returns the final response.
func (p *SubscriptionProxy) Process(ctx context.Context, req subscriptions.YouTubeRequest) (SubscriptionResult, error) {
	if p == nil {
		return SubscriptionResult{}, &ProxyError{Status: http.StatusInternalServerError, Err: errors.New("subscription proxy unconfigured")}
	}
	client := p.httpClient()
	defaults := subscriptionDefaults{
		hubURL:      p.hubURL,
		callbackURL: p.callbackURL,
		verifyMode:  p.verifyMode,
		lease:       p.leaseSeconds,
	}
	applySubscriptionDefaults(&req, p.mode, defaults)

	resp, body, finalReq, err := subscriptions.SubscribeYouTube(ctx, client, p.logger, req)
	if err != nil {
		if resp == nil {
			status := http.StatusBadGateway
			if errors.Is(err, subscriptions.ErrValidation) {
				status = http.StatusBadRequest
			}
			if p.logger != nil {
				p.logger.Printf("%s hub response: %v", p.mode, err)
			}
			return SubscriptionResult{}, &ProxyError{Status: status, Err: err}
		}
		if p.logger != nil {
			p.logger.Printf("%s hub response: %v", p.mode, err)
		}
	}
	if resp == nil {
		return SubscriptionResult{}, &ProxyError{Status: http.StatusBadGateway, Err: fmt.Errorf("%w: missing hub response", ErrUpstream)}
	}

	p.recordSubscriptionResult(finalReq, req, resp, body)
	return SubscriptionResult{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        responseBody(resp, body),
		StatusText:  resp.Status,
	}, nil
}

func (p *SubscriptionProxy) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	timeout := p.timeout
	if timeout <= 0 {
		timeout = defaultSubscriptionTimeout
	}
	return &http.Client{Timeout: timeout}
}

type subscriptionDefaults struct {
	hubURL      string
	callbackURL string
	verifyMode  string
	lease       int
}

func applySubscriptionDefaults(req *subscriptions.YouTubeRequest, mode string, defaults subscriptionDefaults) {
	req.Mode = mode
	if strings.TrimSpace(req.HubURL) == "" {
		req.HubURL = defaults.hubURL
	}
	if strings.TrimSpace(req.Callback) == "" {
		req.Callback = defaults.callbackURL
	}
	if strings.TrimSpace(req.Verify) == "" {
		req.Verify = defaults.verifyMode
	}
	if strings.EqualFold(mode, "subscribe") && req.LeaseSeconds <= 0 {
		req.LeaseSeconds = defaults.lease
	}
}

func (p *SubscriptionProxy) recordSubscriptionResult(finalReq, originalReq subscriptions.YouTubeRequest, resp *http.Response, body []byte) {
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

func responseBody(resp *http.Response, body []byte) []byte {
	if len(body) > 0 {
		return body
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return []byte(resp.Status)
	}
	return nil
}
