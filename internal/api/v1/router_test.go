package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"live-stream-alerts/config"
	adminauth "live-stream-alerts/internal/admin/auth"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
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
	runtime := RuntimeInfo{Name: "app", Addr: "127.0.0.1", Port: ":1234", ReadTimeout: "1s"}
	dir := t.TempDir()
	router := NewRouter(Options{
		Logger:          logger,
		RuntimeInfo:     runtime,
		StreamersPath:   filepath.Join(dir, "streamers.json"),
		SubmissionsPath: filepath.Join(dir, "submissions.json"),
		AdminAuth:       adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"}),
		YouTube:         testYouTubeConfig(),
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

func TestAdminLoginRoute(t *testing.T) {
	manager := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret", TokenTTL: time.Minute})
	dir := t.TempDir()
	router := NewRouter(Options{
		AdminAuth:       manager,
		StreamersPath:   filepath.Join(dir, "streamers.json"),
		SubmissionsPath: filepath.Join(dir, "submissions.json"),
		YouTube:         testYouTubeConfig(),
	})

	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Fatalf("expected token in response")
	}

	badBody, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "bad"})
	badReq := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(badBody))
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid creds, got %d", badRec.Code)
	}
}

func TestAdminSubmissionsRoutes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "submissions.json")
	streamersPath := filepath.Join(dir, "streamers.json")
	file := submissions.File{
		Submissions: []submissions.Submission{
			{ID: "1", Alias: "Pending", SubmittedAt: time.Now().UTC()},
		},
	}
	data, _ := json.MarshalIndent(file, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write submissions file: %v", err)
	}

	manager := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"})
	token, _ := manager.Login("admin@example.com", "secret")

	router := NewRouter(Options{
		AdminAuth:       manager,
		SubmissionsPath: path,
		StreamersPath:   streamersPath,
		YouTube:         testYouTubeConfig(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/submissions", nil)
	req.Header.Set("Authorization", "Bearer "+token.Value)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	postBody, _ := json.Marshal(map[string]string{"action": "reject", "id": "1"})
	postReq := httptest.NewRequest(http.MethodPost, "/api/admin/submissions", bytes.NewReader(postBody))
	postReq.Header.Set("Authorization", "Bearer "+token.Value)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200 rejecting submission, got %d", postRec.Code)
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
