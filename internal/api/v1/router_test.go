package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"live-stream-alerts/internal/platforms/youtube/api"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
)

type stubLogger struct {
	entries []string
}

func (s *stubLogger) Printf(format string, args ...any) {
	s.entries = append(s.entries, format)
}

func TestNewRouterServesConfigAndRoot(t *testing.T) {
	logger := &stubLogger{}
	runtime := RuntimeInfo{Name: "app", Addr: "127.0.0.1", Port: ":1234", ReadTimeout: "1s"}
	router := NewRouter(Options{Logger: logger, RuntimeInfo: runtime})

	t.Run("root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if body := rr.Body.String(); !strings.Contains(body, "id=\"app-root\"") {
			t.Fatalf("expected UI markup, got %q", body)
		}
	})

	t.Run("config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/server/config", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var decoded RuntimeInfo
		if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if decoded != runtime {
			t.Fatalf("expected %v, got %v", runtime, decoded)
		}
	})

	if len(logger.entries) == 0 {
		t.Fatalf("expected logger to record requests")
	}
}

func TestRespondJSONSetsContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	respondJSON(rr, map[string]string{"key": "value"})
	if rr.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", rr.Header().Get("Content-Type"))
	}
}

func TestNewRouterWithoutLogger(t *testing.T) {
	router := NewRouter(Options{})
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

func TestAlertsRouteRejectsUnsupportedMethods(t *testing.T) {
	router := NewRouter(Options{})
	req := httptest.NewRequest(http.MethodPost, "/alerts", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != http.MethodGet+", "+http.MethodPost {
		t.Fatalf("expected Allow header to advertise GET/POST, got %q", allow)
	}
}

func TestStreamersWatchHandlerSendsChangeEvents(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "streamers.json")
	if err := os.WriteFile(path, []byte(`{"streamers":[]}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/streamers/watch", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	recorder := newSSERecorder()
	done := make(chan struct{})
	go func() {
		handler := streamersWatchHandler(streamersWatchOptions{FilePath: path, PollInterval: 10 * time.Millisecond})
		handler.ServeHTTP(recorder, req)
		close(done)
	}()
	time.Sleep(25 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"streamers":[{"streamer":{"alias":"Test"}}]}`), 0o644); err != nil {
		t.Fatalf("update file: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done
	if recorder.status != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.status)
	}
	if !strings.Contains(recorder.buf.String(), "event: change") {
		t.Fatalf("expected change event, got %q", recorder.buf.String())
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
	statusClient := &fakeStatusClient{statuses: map[string]api.LiveStatus{
		"abc123": {VideoID: "abc123", IsLive: true, IsLiveNow: true, PlayabilityStatus: "OK"},
	}}

	router := NewRouter(Options{
		Logger:        logger,
		StreamersPath: streamersPath,
		AlertNotifications: youtubehandlers.NotificationOptions{
			Logger:       logger,
			StatusClient: statusClient,
		},
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
	if statusClient.calls != 1 {
		t.Fatalf("expected live status client to be called once, got %d", statusClient.calls)
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

type fakeStatusClient struct {
	statuses map[string]api.LiveStatus
	calls    int
}

type sseRecorder struct {
	headers http.Header
	buf     bytes.Buffer
	status  int
}

func newSSERecorder() *sseRecorder {
	return &sseRecorder{headers: make(http.Header)}
}

func (r *sseRecorder) Header() http.Header { return r.headers }

func (r *sseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.buf.Write(b)
}

func (r *sseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (r *sseRecorder) Flush() {}

func (f *fakeStatusClient) LiveStatus(ctx context.Context, videoID string) (api.LiveStatus, error) {
	f.calls++
	if status, ok := f.statuses[videoID]; ok {
		return status, nil
	}
	return api.LiveStatus{}, fmt.Errorf("unknown video %s", videoID)
}
