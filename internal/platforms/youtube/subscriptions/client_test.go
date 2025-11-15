package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-stream-alerts/internal/platforms/youtube/websub"
)

func TestNormaliseSubscribeRequest(t *testing.T) {
	req := YouTubeRequest{}
	NormaliseSubscribeRequest(&req)
	if req.Callback != DefaultCallbackURL || req.Mode != DefaultMode || req.LeaseSeconds != DefaultLease {
		t.Fatalf("unexpected defaults applied: %#v", req)
	}
}

func TestNormaliseSubscribeRequestDoesNotOverrideProvidedValues(t *testing.T) {
	req := YouTubeRequest{
		HubURL:       "https://custom-hub",
		Callback:     "https://custom-callback",
		Verify:       "sync",
		LeaseSeconds: 10,
	}
	NormaliseSubscribeRequest(&req)
	if req.HubURL != "https://custom-hub" {
		t.Fatalf("hub url was overridden: %s", req.HubURL)
	}
	if req.Callback != "https://custom-callback" {
		t.Fatalf("callback was overridden: %s", req.Callback)
	}
	if req.Verify != "sync" {
		t.Fatalf("verify was overridden: %s", req.Verify)
	}
	if req.LeaseSeconds != 10 {
		t.Fatalf("lease was overridden: %d", req.LeaseSeconds)
	}
	if req.Mode != DefaultMode {
		t.Fatalf("mode should still be forced to %s", DefaultMode)
	}
}

func TestNormaliseUnsubscribeRequestDoesNotOverrideProvidedValues(t *testing.T) {
	req := YouTubeRequest{
		HubURL:       "https://custom-hub",
		Callback:     "https://custom-callback",
		Verify:       "sync",
		LeaseSeconds: 10,
	}
	NormaliseUnsubscribeRequest(&req)
	if req.HubURL != "https://custom-hub" {
		t.Fatalf("hub url was overridden: %s", req.HubURL)
	}
	if req.Callback != "https://custom-callback" {
		t.Fatalf("callback was overridden: %s", req.Callback)
	}
	if req.Verify != "sync" {
		t.Fatalf("verify was overridden: %s", req.Verify)
	}
	if req.LeaseSeconds != 10 {
		t.Fatalf("lease was overridden: %d", req.LeaseSeconds)
	}
	if req.Mode != "unsubscribe" {
		t.Fatalf("mode should be unsubscribe, got %s", req.Mode)
	}
}

func TestSubscribeYouTubeSuccess(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("hub.mode") != "subscribe" {
			t.Fatalf("expected subscribe mode")
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer hub.Close()

	req := YouTubeRequest{Topic: "https://example", Callback: hub.URL, Verify: "async", VerifyToken: "token", ChannelID: "UC1"}
	resp, body, finalReq, err := SubscribeYouTube(context.Background(), hub.Client(), hub.URL, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202")
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body %q", string(body))
	}
	if _, ok := websub.LookupExpectation(finalReq.VerifyToken); !ok {
		t.Fatalf("expected expectation to be registered")
	}
	websub.CancelExpectation(finalReq.VerifyToken)
}

func TestSubscribeYouTubeValidatesInputs(t *testing.T) {
	if _, _, _, err := SubscribeYouTube(context.Background(), nil, "", YouTubeRequest{}); err == nil {
		t.Fatalf("expected error for missing hub url")
	}
	if _, _, _, err := SubscribeYouTube(context.Background(), nil, "http://hub", YouTubeRequest{}); err == nil {
		t.Fatalf("expected error for missing topic")
	}
}

func TestSubscribeYouTubePropagatesHubErrors(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer hub.Close()

	req := YouTubeRequest{Topic: "https://example", Callback: hub.URL, Verify: "async", VerifyToken: "token"}
	resp, body, _, err := SubscribeYouTube(context.Background(), hub.Client(), hub.URL, req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected response returned")
	}
	if string(body) != "bad" {
		t.Fatalf("expected body from hub")
	}
}
