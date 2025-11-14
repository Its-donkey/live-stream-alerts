package subscriptions

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestSubscribeSkipsWhenNoYouTubePlatform(t *testing.T) {
	if err := Subscribe(context.Background(), streamers.Record{}, Options{}); err != nil {
		t.Fatalf("expected nil error when youtube config missing")
	}
}

func TestSubscribeValidatesChannelID(t *testing.T) {
	rt := mockRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("fail")), Header: make(http.Header)}, nil
	})
	record := streamers.Record{
		Streamer:  streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{Handle: "@test"}},
	}
	if err := Subscribe(context.Background(), record, Options{Client: &http.Client{Transport: rt}}); err == nil {
		t.Fatalf("expected error when channel id cannot be resolved")
	}
}

func TestSubscribeSuccess(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID: "UC123",
			Handle:    "@test",
		}},
	}
	logger := &capturingLogger{}
	if err := Subscribe(context.Background(), record, Options{Client: hub.Client(), HubURL: hub.URL, Logger: logger}); err != nil {
		t.Fatalf("subscribe returned error: %v", err)
	}
	if len(logger.messages) == 0 {
		t.Fatalf("expected log entry")
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

	record := streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			Handle: "example",
		}},
	}

	if err := Subscribe(context.Background(), record, Options{Client: client, HubURL: "https://hub"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
