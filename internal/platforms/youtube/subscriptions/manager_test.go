package subscriptions

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.verify"); got != "async" {
			t.Fatalf("expected verify async, got %s", got)
		}
		if got := r.Form.Get("hub.lease_seconds"); got != "60" {
			t.Fatalf("expected lease seconds 60, got %s", got)
		}
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

func TestSubscribeUsesStoredLeaseSeconds(t *testing.T) {
	const storedLease = 7200
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.lease_seconds"); got != strconv.Itoa(storedLease) {
			t.Fatalf("expected lease seconds %d, got %s", storedLease, got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	configureTestYouTube(t, hub.URL)
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID:    "UC999",
			LeaseSeconds: storedLease,
		}},
	}

	if err := ManageSubscription(context.Background(), record, Options{Client: hub.Client(), HubURL: hub.URL, Mode: "subscribe"}); err != nil {
		t.Fatalf("subscribe returned error: %v", err)
	}
}

func TestSubscribeUsesOptionLeaseSecondsWhenRecordMissing(t *testing.T) {
	const configuredLease = 864000
	const requestLease = 3600
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.lease_seconds"); got != strconv.Itoa(requestLease) {
			t.Fatalf("expected lease seconds %d, got %s", requestLease, got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	configureTestYouTube(t, hub.URL)
	config.YT.LeaseSeconds = configuredLease
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID: "UC777",
		}},
	}

	if err := ManageSubscription(
		context.Background(),
		record,
		Options{Client: hub.Client(), HubURL: hub.URL, Mode: "subscribe", LeaseSeconds: requestLease},
	); err != nil {
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

func TestUnsubscribeOmitsLeaseSeconds(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.lease_seconds"); got != "" {
			t.Fatalf("expected lease seconds omitted, got %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	configureTestYouTube(t, hub.URL)
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID: "UC123",
		}},
	}

	if err := ManageSubscription(context.Background(), record, Options{Client: hub.Client(), HubURL: hub.URL, Mode: "unsubscribe"}); err != nil {
		t.Fatalf("unsubscribe returned error: %v", err)
	}
}

func TestUnsubscribeUsesStoredContext(t *testing.T) {
	const (
		customTopic    = "https://feeds.example.com/custom"
		storedCallback = "https://callback.old.example.com/hook"
	)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.callback"); got != storedCallback {
			t.Fatalf("expected callback %s, got %s", storedCallback, got)
		}
		if got := r.Form.Get("hub.topic"); got != customTopic {
			t.Fatalf("expected topic %s, got %s", customTopic, got)
		}
		if got := r.Form.Get("hub.verify"); got != "sync" {
			t.Fatalf("expected verify sync, got %s", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	configureTestYouTube(t, "https://config-hub.example.com")
	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID:   "UC123",
			HubSecret:   "secret",
			Topic:       customTopic,
			CallbackURL: storedCallback,
			HubURL:      hub.URL,
			VerifyMode:  "sync",
		}},
	}

	if err := ManageSubscription(
		context.Background(),
		record,
		Options{Client: hub.Client(), HubURL: "https://ignored", Mode: "unsubscribe"},
	); err != nil {
		t.Fatalf("unsubscribe returned error: %v", err)
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
