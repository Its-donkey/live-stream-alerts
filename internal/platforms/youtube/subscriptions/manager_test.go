package subscriptions

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"live-stream-alerts/config"
	"live-stream-alerts/internal/streamers"
)

type capturingLogger struct {
	messages []string
}

func (c *capturingLogger) Printf(format string, args ...any) {
	c.messages = append(c.messages, format)
}

type mockRoundTrip func(*http.Request) (*http.Response, error)

func (m mockRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func configureTestYouTube(t *testing.T, hubURL string) {
	t.Helper()
	original := config.YT
	config.YT = config.YouTubeConfig{
		HubURL:       hubURL,
		CallbackURL:  "https://callback.example.com/alerts",
		LeaseSeconds: 60,
		Mode:         "subscribe",
		Verify:       "async",
	}
	t.Cleanup(func() { config.YT = original })
}

func TestSubscribeSkipsWhenNoYouTubePlatform(t *testing.T) {
	if err := ManageSubscription(context.Background(), streamers.Record{}, Options{}); err != nil {
		t.Fatalf("expected nil error when youtube config missing")
	}
}

func TestSubscribeValidatesChannelID(t *testing.T) {
	rt := mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("fail")), Header: make(http.Header)}, nil
	})
	configureTestYouTube(t, "https://hub.invalid")
	record := streamers.Record{
		Streamer:  streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{Handle: "@test"}},
	}
	if err := ManageSubscription(context.Background(), record, Options{Client: &http.Client{Transport: rt}, Mode: "subscribe"}); err == nil {
		t.Fatalf("expected error when channel id cannot be resolved")
	}
}

func TestSubscribeSuccess(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	configureTestYouTube(t, hub.URL)
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID: "UC123",
			Handle:    "@test",
		}},
	}
	if err := ManageSubscription(context.Background(), record, Options{Client: hub.Client(), HubURL: hub.URL, Mode: "subscribe"}); err != nil {
		t.Fatalf("subscribe returned error: %v", err)
	}
}

func TestSubscribeResolvesChannelIDFromHandle(t *testing.T) {
	client := &http.Client{Transport: mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.youtube.com" {
			body := `{"channelId":"UC1234567890123456789012"}`
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
			return resp, nil
		}
		resp := &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}
		return resp, nil
	})}

	configureTestYouTube(t, "https://hub")
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			Handle: "example",
		}},
	}

	if err := ManageSubscription(context.Background(), record, Options{Client: client, Mode: "subscribe"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubscribeRequiresMode(t *testing.T) {
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID: "UC123",
		}},
	}

	err := ManageSubscription(context.Background(), record, Options{})
	if err == nil {
		t.Fatalf("expected error when mode missing")
	}
	if !strings.Contains(err.Error(), "mode is required") {
		t.Fatalf("expected mode error, got %v", err)
	}
}
