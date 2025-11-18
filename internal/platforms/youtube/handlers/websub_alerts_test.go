package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
)

type memoryLogger struct {
	msgs []string
}

func (l *memoryLogger) Printf(format string, args ...any) {
	l.msgs = append(l.msgs, format)
}

func TestHandleSubscriptionConfirmationSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	_, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{
			Alias:     "Test",
			FirstName: "T",
			LastName:  "User",
			Email:     "test@example.com",
		},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{ChannelID: "UC123", Handle: "@test"}},
	})
	if err != nil {
		t.Fatalf("append streamer: %v", err)
	}

	token := "token"
	challenge := "challenge"
	topic := "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123"
	websub.RegisterExpectation(websub.Expectation{VerifyToken: token, Topic: topic, Mode: "subscribe", LeaseSeconds: 60})
	t.Cleanup(func() { websub.CancelExpectation(token) })

	values := url.Values{}
	values.Set("hub.challenge", challenge)
	values.Set("hub.verify_token", token)
	values.Set("hub.topic", topic)
	values.Set("hub.lease_seconds", "60")
	values.Set("hub.mode", "subscribe")

	req := httptest.NewRequest(http.MethodGet, "/alerts?"+values.Encode(), nil)
	rr := httptest.NewRecorder()

	handled := HandleSubscriptionConfirmation(rr, req, SubscriptionConfirmationOptions{StreamersPath: path, Logger: &memoryLogger{}})
	if !handled {
		t.Fatalf("expected request to be handled")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != challenge {
		t.Fatalf("expected challenge echoed")
	}
	if _, ok := websub.LookupExpectation(token); ok {
		t.Fatalf("expected expectation to be consumed")
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list streamers: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record")
	}
	if records[0].Platforms.YouTube.HubLeaseDate == "" {
		t.Fatalf("expected lease renewal timestamp to be set")
	}
}

func TestHandleSubscriptionConfirmationSkipsLeaseForUnsubscribe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	_, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{
			Alias:     "Test",
			FirstName: "T",
			LastName:  "User",
			Email:     "test@example.com",
		},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{ChannelID: "UC123", Handle: "@test"}},
	})
	if err != nil {
		t.Fatalf("append streamer: %v", err)
	}

	token := "token-unsub"
	challenge := "challenge"
	topic := "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123"
	websub.RegisterExpectation(websub.Expectation{VerifyToken: token, Topic: topic, Mode: "unsubscribe", LeaseSeconds: 60})
	t.Cleanup(func() { websub.CancelExpectation(token) })

	values := url.Values{}
	values.Set("hub.challenge", challenge)
	values.Set("hub.verify_token", token)
	values.Set("hub.topic", topic)
	values.Set("hub.mode", "unsubscribe")

	req := httptest.NewRequest(http.MethodGet, "/alerts?"+values.Encode(), nil)
	rr := httptest.NewRecorder()

	handled := HandleSubscriptionConfirmation(rr, req, SubscriptionConfirmationOptions{StreamersPath: path, Logger: &memoryLogger{}})
	if !handled {
		t.Fatalf("expected request to be handled")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list streamers: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record")
	}
	if records[0].Platforms.YouTube.HubLeaseDate != "" {
		t.Fatalf("expected lease renewal timestamp to remain empty for unsubscribe")
	}
}

func TestHandleSubscriptionConfirmationValidatesRequests(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/alert", nil)
	if HandleSubscriptionConfirmation(rr, req, SubscriptionConfirmationOptions{}) {
		t.Fatalf("expected non-GET to be ignored")
	}

	req = httptest.NewRequest(http.MethodGet, "/ignored", nil)
	if HandleSubscriptionConfirmation(rr, req, SubscriptionConfirmationOptions{}) {
		t.Fatalf("expected non-alert path to be ignored")
	}

	req = httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rr = httptest.NewRecorder()
	if !HandleSubscriptionConfirmation(rr, req, SubscriptionConfirmationOptions{}) {
		t.Fatalf("expected request to be handled")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing challenge")
	}
}

func TestHandleSubscriptionConfirmationMissingExpectation(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/alerts?hub.challenge=a&hub.verify_token=missing", nil)
	rr := httptest.NewRecorder()
	handled := HandleSubscriptionConfirmation(rr, req, SubscriptionConfirmationOptions{})
	if !handled {
		t.Fatalf("expected handled true")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request when expectation missing")
	}
}
