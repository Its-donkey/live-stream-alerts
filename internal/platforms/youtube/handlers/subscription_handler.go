package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

// SubscriptionHandlerOptions configures handlers that talk to the YouTube hub.
type SubscriptionHandlerOptions struct {
	Client       *http.Client
	Logger       logging.Logger
	HubURL       string
	CallbackURL  string
	VerifyMode   string
	LeaseSeconds int
	Proxy        subscriptionProxy
}

// SubscribeHandlerOptions configures the subscribe handler.
type SubscribeHandlerOptions = SubscriptionHandlerOptions

// UnsubscribeHandlerOptions configures the unsubscribe handler.
type UnsubscribeHandlerOptions = SubscriptionHandlerOptions

type subscriptionProxy interface {
	Process(ctx context.Context, req subscriptions.YouTubeRequest) (youtubeservice.SubscriptionResult, error)
}

type subscriptionHandler struct {
	proxy  subscriptionProxy
	logger logging.Logger
}

// NewSubscribeHandler returns an http.Handler that accepts POST requests and forwards them to YouTube's hub.
func NewSubscribeHandler(opts SubscribeHandlerOptions) http.Handler {
	return newSubscriptionHandler("subscribe", opts)
}

// NewUnsubscribeHandler returns an http.Handler that issues unsubscribe requests to YouTube's hub.
func NewUnsubscribeHandler(opts UnsubscribeHandlerOptions) http.Handler {
	return newSubscriptionHandler("unsubscribe", opts)
}

func newSubscriptionHandler(mode string, opts SubscriptionHandlerOptions) http.Handler {
	proxy := opts.Proxy
	if proxy == nil {
		proxy = youtubeservice.NewSubscriptionProxy(mode, youtubeservice.SubscriptionProxyOptions{
			Client:       opts.Client,
			Logger:       opts.Logger,
			HubURL:       strings.TrimSpace(opts.HubURL),
			CallbackURL:  strings.TrimSpace(opts.CallbackURL),
			VerifyMode:   strings.TrimSpace(opts.VerifyMode),
			LeaseSeconds: opts.LeaseSeconds,
		})
	}
	handler := subscriptionHandler{proxy: proxy, logger: opts.Logger}
	return http.HandlerFunc(handler.ServeHTTP)
}

func (h subscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.proxy.Process(r.Context(), req)
	if err != nil {
		h.respondError(w, err)
		return
	}
	writeSubscriptionResponse(w, result)
}

func (h subscriptionHandler) respondError(w http.ResponseWriter, err error) {
	var proxyErr *youtubeservice.ProxyError
	if errors.As(err, &proxyErr) {
		http.Error(w, "hub request failed", proxyErr.Status)
		return
	}
	if h.logger != nil {
		h.logger.Printf("subscription request failed: %v", err)
	}
	http.Error(w, "hub request failed", http.StatusInternalServerError)
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

func writeSubscriptionResponse(w http.ResponseWriter, result youtubeservice.SubscriptionResult) {
	if ct := result.ContentType; ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(result.StatusCode)
	if len(result.Body) > 0 {
		_, _ = w.Write(result.Body)
		return
	}
	if result.StatusCode >= 200 && result.StatusCode < 300 && result.StatusText != "" {
		_, _ = io.WriteString(w, result.StatusText)
	}
}
