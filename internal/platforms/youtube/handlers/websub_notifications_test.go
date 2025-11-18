package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/streamers"
)

type stubVideoLookup struct {
	infos map[string]liveinfo.VideoInfo
	err   error
	calls int
}

func (s *stubVideoLookup) Fetch(ctx context.Context, videoIDs []string) (map[string]liveinfo.VideoInfo, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.infos, nil
}

func TestHandleAlertNotificationUpdatesStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	_, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{Alias: "Test"},
		Platforms: streamers.Platforms{
			YouTube: &streamers.YouTubePlatform{ChannelID: "UCdemo"},
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	body := `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
 <entry>
  <yt:videoId>fbfHCxvsny0</yt:videoId>
  <yt:channelId>UCdemo</yt:channelId>
  <title>Testing 1234</title>
  <published>2025-11-16T09:02:38+00:00</published>
  <updated>2025-11-16T09:02:41+00:00</updated>
 </entry>
</feed>`

	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	started := time.Date(2025, 11, 16, 9, 2, 41, 0, time.UTC)
	lookup := &stubVideoLookup{
		infos: map[string]liveinfo.VideoInfo{
			"fbfHCxvsny0": {
				ID:                   "fbfHCxvsny0",
				ChannelID:            "UCdemo",
				LiveBroadcastContent: "live",
				ActualStartTime:      started,
			},
		},
	}
	opts := AlertNotificationOptions{
		StreamersPath: path,
		VideoLookup:   lookup,
	}

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to process request")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if lookup.calls != 1 {
		t.Fatalf("expected lookup to be called once, got %d", lookup.calls)
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 || !records[0].Status.Live {
		t.Fatalf("expected live status to be set")
	}
	if records[0].Status.YouTube == nil || records[0].Status.YouTube.VideoID != "fbfHCxvsny0" {
		t.Fatalf("expected youtube status to be populated")
	}
}

func TestHandleAlertNotificationRejectsInvalidFeed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString("not xml"))
	rr := httptest.NewRecorder()
	opts := AlertNotificationOptions{VideoLookup: &stubVideoLookup{}}

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to run")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleAlertNotificationHandlesLookupFailure(t *testing.T) {
	body := `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
 <entry>
  <yt:videoId>abc123</yt:videoId>
  <yt:channelId>UCdemo</yt:channelId>
  <title>Test</title>
 </entry>
</feed>`
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	opts := AlertNotificationOptions{VideoLookup: &stubVideoLookup{err: errors.New("lookup failed")}}

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to run")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 when lookup fails, got %d", rr.Code)
	}
}

func TestHandleAlertNotificationSkipsUnsupportedPaths(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/other", nil)
	if HandleAlertNotification(httptest.NewRecorder(), req, AlertNotificationOptions{VideoLookup: &stubVideoLookup{}}) {
		t.Fatalf("expected handler to ignore unsupported paths")
	}
}
