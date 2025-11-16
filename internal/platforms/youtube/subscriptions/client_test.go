package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-stream-alerts/config"
	"live-stream-alerts/internal/platforms/youtube/websub"
)

func withConfig(t *testing.T, cfg config.YouTubeConfig) {
	t.Helper()
	original := config.YT
	config.YT = cfg
	t.Cleanup(func() { config.YT = original })
}

func TestSubscribeYouTubeSuccess(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.mode"); got != "subscribe" {
			t.Fatalf("expected subscribe mode, got %s", got)
		}
		if got := r.Form.Get("hub.callback"); got != "https://callback.example.com/alerts" {
			t.Fatalf("callback mismatch: %s", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer hub.Close()

	withConfig(t, config.YouTubeConfig{
		HubURL:       hub.URL,
		CallbackURL:  "https://callback.example.com/alerts",
		LeaseSeconds: 120,
		Verify:       "async",
	})

	req := YouTubeRequest{
		Topic:        "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123",
		Mode:         "subscribe",
		Verify:       "async",
		LeaseSeconds: 120,
	}

	resp, body, finalReq, err := SubscribeYouTube(context.Background(), hub.Client(), nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", string(body))
	}
	if _, ok := websub.LookupExpectation(finalReq.VerifyToken); !ok {
		t.Fatalf("expected expectation to be registered")
	}
	websub.CancelExpectation(finalReq.VerifyToken)
}

func TestSubscribeYouTubeValidatesConfig(t *testing.T) {
	withConfig(t, config.YouTubeConfig{}) // everything empty
	_, _, _, err := SubscribeYouTube(context.Background(), nil, nil, YouTubeRequest{
		Topic: "https://example",
		Mode:  "subscribe",
	})
	if err == nil {
		t.Fatalf("expected error when hub url is missing")
	}
}

func TestSubscribeYouTubePropagatesHubErrors(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer hub.Close()

	withConfig(t, config.YouTubeConfig{
		HubURL:       hub.URL,
		CallbackURL:  "https://callback.example.com/alerts",
		LeaseSeconds: 60,
		Verify:       "async",
	})

	req := YouTubeRequest{
		Topic:        "https://example",
		Mode:         "subscribe",
		LeaseSeconds: 60,
	}

	resp, body, _, err := SubscribeYouTube(context.Background(), hub.Client(), nil, req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected response with 400")
	}
	if string(body) != "bad" {
		t.Fatalf("expected hub body propagated")
	}
}
