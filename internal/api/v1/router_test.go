package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	if allow := rr.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header to advertise GET, got %q", allow)
	}
}
