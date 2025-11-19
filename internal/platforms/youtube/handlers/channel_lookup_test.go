package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
)

type stubChannelResolver struct {
	id   string
	err  error
	seen string
}

func (s *stubChannelResolver) ResolveHandle(_ context.Context, handle string) (string, error) {
	s.seen = handle
	return s.id, s.err
}

func TestChannelLookupHandler(t *testing.T) {
	resolver := &stubChannelResolver{id: "UC1234567890123456789012"}
	handler := NewChannelLookupHandler(ChannelLookupHandlerOptions{Resolver: resolver})

	t.Run("method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/youtube/channel", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rr.Code)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewBufferString("{"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{"handle": "@example"})
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewReader(payload))
		rr := httptest.NewRecorder()
		resolver.err = fmt.Errorf("%w: handle required", youtubeservice.ErrValidation)
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("upstream error", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{"handle": "@example"})
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewReader(payload))
		rr := httptest.NewRecorder()
		resolver.err = fmt.Errorf("%w: boom", youtubeservice.ErrUpstream)
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rr.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		resolver.err = nil
		payload, _ := json.Marshal(map[string]string{"handle": "@example"})
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewReader(payload))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !bytes.Contains(rr.Body.Bytes(), []byte("UC12345678901234567890")) {
			t.Fatalf("expected response to contain channel ID")
		}
		if resolver.seen != "@example" {
			t.Fatalf("expected resolver to receive handle")
		}
	})
}
