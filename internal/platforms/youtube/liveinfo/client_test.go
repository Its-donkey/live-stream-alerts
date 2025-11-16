package liveinfo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "abc123,def456"; r.URL.Query().Get("id") != want {
			t.Fatalf("expected ids %s, got %s", want, r.URL.Query().Get("id"))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{
			"items":[
				{
					"id":"abc123",
					"snippet":{
						"channelId":"UCdemo",
						"title":"Live demo",
						"liveBroadcastContent":"live"
					},
					"liveStreamingDetails":{
						"actualStartTime":"2025-11-16T09:02:41Z"
					}
				}
			]
		}`)
	}))
	defer server.Close()

	client := &Client{
		APIKey:     "key",
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
	}
	info, err := client.Fetch(context.Background(), []string{"abc123", "def456"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(info) != 1 {
		t.Fatalf("expected one entry, got %d", len(info))
	}
	entry := info["abc123"]
	if !entry.IsLive() {
		t.Fatalf("expected entry to be live")
	}
	if entry.ChannelID != "UCdemo" {
		t.Fatalf("unexpected channel id %q", entry.ChannelID)
	}
	if entry.ActualStartTime.IsZero() {
		t.Fatalf("expected start time to be parsed")
	}
}

func TestClientMissingKey(t *testing.T) {
	client := &Client{}
	if _, err := client.Fetch(context.Background(), []string{"abc"}); err == nil {
		t.Fatalf("expected error for missing api key")
	}
}

func TestVideoInfoIsLive(t *testing.T) {
	if (VideoInfo{LiveBroadcastContent: "live"}).IsLive() == false {
		t.Fatalf("expected live content to be detected")
	}
	if (VideoInfo{ActualStartTime: time.Now()}).IsLive() == false {
		t.Fatalf("expected actual start to imply live")
	}
	if (VideoInfo{}).IsLive() {
		t.Fatalf("expected zero value to be not live")
	}
}
