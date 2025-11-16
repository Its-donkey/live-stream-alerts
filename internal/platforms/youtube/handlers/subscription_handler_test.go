package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"live-stream-alerts/config"
)

func configureSubscriptionDefaults(t *testing.T) {
	t.Helper()
	original := config.YT
	config.YT = config.YouTubeConfig{
		HubURL:       "https://hub.example.com/subscribe",
		CallbackURL:  "https://callback.example.com/alerts",
		LeaseSeconds: 60,
		Mode:         "subscribe",
		Verify:       "async",
	}
	t.Cleanup(func() { config.YT = original })
}

type stubRoundTrip func(*http.Request) (*http.Response, error)

func (s stubRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return s(req)
}

func TestSubscriptionHandlerReturnsBadRequestForValidationErrors(t *testing.T) {
	configureSubscriptionDefaults(t)

	handler := NewUnsubscribeHandler(UnsubscribeHandlerOptions{})
	body, _ := json.Marshal(map[string]string{
		"topic": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/unsubscribe", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation error, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "topic is required") {
		t.Fatalf("expected validation message, got %q", rr.Body.String())
	}
}

func TestSubscriptionHandlerReturnsBadGatewayForHubFailures(t *testing.T) {
	configureSubscriptionDefaults(t)

	client := &http.Client{Transport: stubRoundTrip(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: i/o timeout")
	})}

	handler := NewSubscribeHandler(SubscribeHandlerOptions{Client: client})
	body, _ := json.Marshal(map[string]string{
		"topic": "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/subscribe", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for hub failure, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "post to hub") {
		t.Fatalf("expected hub failure message, got %q", rr.Body.String())
	}
}

func TestUnsubscribeHandlerDoesNotSetLeaseSeconds(t *testing.T) {
	configureSubscriptionDefaults(t)

	var leaseParam string
	client := &http.Client{Transport: stubRoundTrip(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		leaseParam = req.Form.Get("hub.lease_seconds")
		resp := &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(strings.NewReader("accepted")),
			Header:     make(http.Header),
		}
		return resp, nil
	})}

	handler := NewUnsubscribeHandler(UnsubscribeHandlerOptions{Client: client})
	body, _ := json.Marshal(map[string]string{
		"topic": "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/unsubscribe", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if leaseParam != "" {
		t.Fatalf("expected lease seconds to be omitted for unsubscribe, got %q", leaseParam)
	}
}
