package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"live-stream-alerts/internal/platforms/youtube/api"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	"live-stream-alerts/internal/platforms/youtube/websub"
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
		if rr.Body.String() != "UI assets not configured" {
			t.Fatalf("unexpected body %q", rr.Body.String())
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

func TestAlertsRouteProcessesNotifications(t *testing.T) {
	tmp := t.TempDir()
	streamersPath := filepath.Join(tmp, "streamers.json")
	if err := os.WriteFile(streamersPath, []byte(`{"$schema":"","streamers":[]}`), 0o644); err != nil {
		t.Fatalf("write streamers file: %v", err)
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
}

type fakeStatusClient struct {
	statuses map[string]api.LiveStatus
	calls    int
}

func (f *fakeStatusClient) LiveStatus(ctx context.Context, videoID string) (api.LiveStatus, error) {
	f.calls++
	if status, ok := f.statuses[videoID]; ok {
		return status, nil
	}
	return api.LiveStatus{}, fmt.Errorf("unknown video %s", videoID)
}
