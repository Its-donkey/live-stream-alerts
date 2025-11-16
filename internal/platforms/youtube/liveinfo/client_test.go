package liveinfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetchParsesLivePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/watch" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("v") == "" {
			t.Fatalf("missing video id")
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><head><script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"abc123","channelId":"UCdemo","title":"Live demo","isLiveContent":true},"microformat":{"playerMicroformatRenderer":{"liveBroadcastDetails":{"startTimestamp":"2025-11-16T09:02:41Z"}}}};;</script></head><body></body></html>`))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL + "/watch",
	}

	info, err := client.Fetch(context.Background(), []string{"abc123"})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	entry, ok := info["abc123"]
	if !ok {
		t.Fatalf("expected entry for video")
	}
	if !entry.IsLive() {
		t.Fatalf("expected entry to be live")
	}
	if entry.ChannelID != "UCdemo" {
		t.Fatalf("unexpected channel id %q", entry.ChannelID)
	}
	if entry.ActualStartTime.IsZero() {
		t.Fatalf("expected start timestamp to be parsed")
	}
}

func TestClientFetchSkipsFailures(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "bad", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"def456","channelId":"UCdemo","title":"Demo","isLiveContent":false}};</script>`))
	}))
	defer server.Close()

	client := &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL + "/watch",
	}

	info, err := client.Fetch(context.Background(), []string{"abc123", "def456"})
	if err != nil {
		t.Fatalf("expected partial success, got error %v", err)
	}
	if len(info) != 1 {
		t.Fatalf("expected one successful entry, got %d", len(info))
	}
}

func TestVideoInfoIsLive(t *testing.T) {
	if !(VideoInfo{LiveBroadcastContent: "live"}).IsLive() {
		t.Fatalf("expected live content to be true")
	}
	if !(VideoInfo{ActualStartTime: time.Now()}).IsLive() {
		t.Fatalf("expected actual start time to imply live")
	}
	if (VideoInfo{}).IsLive() {
		t.Fatalf("expected zero value to be offline")
	}
}
