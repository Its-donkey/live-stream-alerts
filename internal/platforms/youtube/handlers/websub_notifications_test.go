package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"live-stream-alerts/internal/platforms/youtube/api"
)

func TestHandleNotificationChecksLiveStatus(t *testing.T) {
	logger := &memoryLogger{}
	client := &stubLiveStatusClient{statuses: map[string]api.LiveStatus{
		"abc": {VideoID: "abc", IsLive: true, IsLiveNow: true, PlayabilityStatus: "OK"},
	}}
	req := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader(sampleNotificationBody("abc")))
	resp := httptest.NewRecorder()

	handled := HandleNotification(resp, req, NotificationOptions{Logger: logger, StatusClient: client})
	if !handled {
		t.Fatalf("expected notification to be handled")
	}
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.Code)
	}
	if client.calls != 1 {
		t.Fatalf("expected one status call, got %d", client.calls)
	}
	if len(logger.msgs) == 0 {
		t.Fatalf("expected log entries")
	}
}

func TestHandleNotificationRejectsInvalidFeed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader("<feed>broken"))
	resp := httptest.NewRecorder()

	handled := HandleNotification(resp, req, NotificationOptions{})
	if !handled {
		t.Fatalf("expected handler to run")
	}
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestHandleNotificationEnforcesBodyLimit(t *testing.T) {
	body := strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader(body))
	resp := httptest.NewRecorder()

	HandleNotification(resp, req, NotificationOptions{BodyLimit: 10})
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.Code)
	}
}

type stubLiveStatusClient struct {
	statuses map[string]api.LiveStatus
	calls    int
}

func (s *stubLiveStatusClient) LiveStatus(ctx context.Context, videoID string) (api.LiveStatus, error) {
	s.calls++
	return s.statuses[videoID], nil
}

func sampleNotificationBody(videoID string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
 <entry>
  <yt:videoId>` + videoID + `</yt:videoId>
  <yt:channelId>UC123</yt:channelId>
  <title>Live</title>
 </entry>
</feed>`
}
