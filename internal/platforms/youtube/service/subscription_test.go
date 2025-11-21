package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

type stubTransport func(*http.Request) (*http.Response, error)

func (s stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return s(req)
}

func TestSubscriptionProxySuccess(t *testing.T) {
	client := &http.Client{Transport: stubTransport(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if req.Form.Get("hub.mode") != "subscribe" {
			t.Fatalf("expected subscribe mode")
		}
		resp := &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString("accepted")),
		}
		resp.Header.Set("Content-Type", "text/plain")
		return resp, nil
	})}
	proxy := NewSubscriptionProxy("subscribe", SubscriptionProxyOptions{
		Client:       client,
		HubURL:       "https://hub.example.com",
		CallbackURL:  "https://callback.example.com",
		VerifyMode:   "async",
		LeaseSeconds: 60,
	})
	result, err := proxy.Process(
		httptest.NewRequest(http.MethodPost, "/", nil).Context(),
		subscriptions.YouTubeRequest{Topic: "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123"},
	)
	if err != nil {
		t.Fatalf("process proxy: %v", err)
	}
	if result.StatusCode != http.StatusAccepted || result.ContentType != "text/plain" || string(result.Body) != "accepted" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestSubscriptionProxyReturnsErrorWhenHubFails(t *testing.T) {
	client := &http.Client{Transport: stubTransport(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: timeout")
	})}
	proxy := NewSubscriptionProxy("subscribe", SubscriptionProxyOptions{Client: client})
	_, err := proxy.Process(
		httptest.NewRequest(http.MethodPost, "/", nil).Context(),
		subscriptions.YouTubeRequest{Topic: "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123"},
	)
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) || proxyErr.Status != http.StatusBadGateway {
		t.Fatalf("expected proxy error with 502, got %v", err)
	}
}

func TestSubscriptionProxyValidatesTopic(t *testing.T) {
	proxy := NewSubscriptionProxy("subscribe", SubscriptionProxyOptions{})
	_, err := proxy.Process(httptest.NewRequest(http.MethodPost, "/", nil).Context(), subscriptions.YouTubeRequest{})
	if err == nil {
		t.Fatalf("expected proxy error")
	}
}

func TestUnsubscribeProxyOmitsLeaseSeconds(t *testing.T) {
	var lease string
	client := &http.Client{Transport: stubTransport(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		lease = req.Form.Get("hub.lease_seconds")
		resp := &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString("accepted")),
		}
		return resp, nil
	})}
	proxy := NewSubscriptionProxy("unsubscribe", SubscriptionProxyOptions{
		Client:      client,
		HubURL:      "https://hub.example.com",
		CallbackURL: "https://callback.example.com",
		VerifyMode:  "async",
	})
	body := subscriptions.YouTubeRequest{Topic: "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123"}
	if _, err := proxy.Process(httptest.NewRequest(http.MethodPost, "/", nil).Context(), body); err != nil {
		t.Fatalf("process proxy: %v", err)
	}
	if lease != "" {
		t.Fatalf("expected lease seconds omitted for unsubscribe, got %q", lease)
	}
}
