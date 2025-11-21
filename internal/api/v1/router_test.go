package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"live-stream-alerts/config"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
)

type stubLogger struct {
	entries []string
}

func (s *stubLogger) Printf(format string, args ...any) {
	s.entries = append(s.entries, format)
}

func testYouTubeConfig() config.YouTubeConfig {
	return config.YouTubeConfig{
		HubURL:       "https://hub.example.com",
		CallbackURL:  "https://callback.example.com/alerts",
		Verify:       "async",
		LeaseSeconds: 60,
	}
}

func TestNewRouterServesConfigAndRoot(t *testing.T) {
	logger := &stubLogger{}
	dir := t.TempDir()
	router := NewRouter(Options{
		Logger:        logger,
		StreamersPath: filepath.Join(dir, "streamers.json"),
		YouTube:       testYouTubeConfig(),
		AlertNotifications: youtubehandlers.AlertNotificationOptions{
			VideoLookup: noopVideoLookup{},
		},
	})

	t.Run("root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if body := rr.Body.String(); body != rootPlaceholder {
			t.Fatalf("expected placeholder response, got %q", body)
		}
	})

	if len(logger.entries) == 0 {
		t.Fatalf("expected logger to record requests")
	}
}

func TestNewRouterWithoutLogger(t *testing.T) {
	router := NewRouter(Options{YouTube: testYouTubeConfig()})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAlertsRouteHandlesVerification(t *testing.T) {
	tmp := t.TempDir()
	streamersPath := filepath.Join(tmp, "streamers.json")
	if err := os.WriteFile(streamersPath, []byte(`{"$schema":"","streamers":[]}`), 0o644); err != nil {
		t.Fatalf("write streamers file: %v", err)
	}

	logger := &stubLogger{}
	router := NewRouter(Options{
		Logger:        logger,
		StreamersPath: streamersPath,
		AlertNotifications: youtubehandlers.AlertNotificationOptions{
			VideoLookup: noopVideoLookup{},
		},
		YouTube: testYouTubeConfig(),
	})

	token := "verify-token"
	websub.RegisterExpectation(websub.Expectation{
		VerifyToken:  token,
		Topic:        "https://example.com/feed",
		Mode:         "subscribe",
		LeaseSeconds: 864000,
	})

	req := httptest.NewRequest(http.MethodGet, "/alerts?hub.mode=subscribe&hub.topic=https://example.com/feed&hub.challenge=abc&hub.lease_seconds=864000&hub.verify_token="+token, nil)
	req.Header.Set("User-Agent", "FeedFetcher-Google")
	req.Header.Set("From", "googlebot(at)googlebot.com")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "abc" {
		t.Fatalf("expected challenge to be echoed, got %q", body)
	}
}

func TestAlertsRoutePostRequiresValidFeed(t *testing.T) {
	tmp := t.TempDir()
	streamersPath := filepath.Join(tmp, "streamers.json")
	if err := os.WriteFile(streamersPath, []byte(`{"$schema":"","streamers":[]}`), 0o644); err != nil {
		t.Fatalf("write streamers file: %v", err)
	}
	router := NewRouter(Options{
		StreamersPath: streamersPath,
		AlertNotifications: youtubehandlers.AlertNotificationOptions{
			VideoLookup: noopVideoLookup{},
		},
		YouTube: testYouTubeConfig(),
	})
	req := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader("not xml"))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != http.MethodGet+", "+http.MethodPost {
		t.Fatalf("expected Allow header to advertise GET/POST, got %q", allow)
	}
}

func TestAlertsRouteProcessesNotifications(t *testing.T) {
	tmp := t.TempDir()
	streamersPath := filepath.Join(tmp, "streamers.json")
	if _, err := streamers.Append(streamersPath, streamers.Record{
		Streamer:  streamers.Streamer{Alias: "Test", Email: "test@example.com"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{ChannelID: "UC123"}},
	}); err != nil {
		t.Fatalf("append streamer: %v", err)
	}

	logger := &stubLogger{}
	lookup := &fakeVideoLookup{responses: map[string]liveinfo.VideoInfo{
		"abc123": {
			ID:                   "abc123",
			ChannelID:            "UC123",
			Title:                "Live show",
			LiveBroadcastContent: "live",
			ActualStartTime:      time.Now(),
		},
	}}

	router := NewRouter(Options{
		Logger:        logger,
		StreamersPath: streamersPath,
		AlertNotifications: youtubehandlers.AlertNotificationOptions{
			Logger:      logger,
			VideoLookup: lookup,
		},
		YouTube: testYouTubeConfig(),
	})

	body := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
 <entry>
  <yt:videoId>abc123</yt:videoId>
  <yt:channelId>UC123</yt:channelId>
  <title>Live show</title>
 </entry>
</feed>`
	req := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader(body))
	req.Header.Set("User-Agent", "FeedFetcher-Google")
	req.Header.Set("From", "googlebot(at)googlebot.com")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.Code)
	}
	if lookup.calls != 1 {
		t.Fatalf("expected live lookup to be called once, got %d", lookup.calls)
	}
	records, err := streamers.List(streamersPath)
	if err != nil {
		t.Fatalf("list streamers: %v", err)
	}
	if records[0].Status == nil || records[0].Status.YouTube == nil || !records[0].Status.YouTube.Live {
		t.Fatalf("expected youtube status to be live: %+v", records[0].Status)
	}
	if records[0].Status.YouTube.VideoID != "abc123" {
		t.Fatalf("expected video id to be recorded, got %q", records[0].Status.YouTube.VideoID)
	}
}

type fakeVideoLookup struct {
	responses map[string]liveinfo.VideoInfo
	calls     int
}

func (f *fakeVideoLookup) Fetch(ctx context.Context, videoIDs []string) (map[string]liveinfo.VideoInfo, error) {
	f.calls++
	out := make(map[string]liveinfo.VideoInfo, len(videoIDs))
	for _, id := range videoIDs {
		if info, ok := f.responses[id]; ok {
			out[id] = info
		}
	}
	return out, nil
}

type noopVideoLookup struct{}

func (noopVideoLookup) Fetch(ctx context.Context, videoIDs []string) (map[string]liveinfo.VideoInfo, error) {
	return map[string]liveinfo.VideoInfo{}, nil
}
