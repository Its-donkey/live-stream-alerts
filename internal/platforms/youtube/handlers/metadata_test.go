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

type stubMetadataFetcher struct {
	data youtubeservice.Metadata
	err  error
	url  string
}

func (s *stubMetadataFetcher) Fetch(ctx context.Context, rawURL string) (youtubeservice.Metadata, error) {
	s.url = rawURL
	return s.data, s.err
}

func TestMetadataHandlerSuccess(t *testing.T) {
	fetcher := &stubMetadataFetcher{
		data: youtubeservice.Metadata{
			Description: "Desc",
			Title:       "Title",
			Handle:      "@example",
			ChannelID:   "UC999",
		},
	}
	handler := NewMetadataHandler(MetadataHandlerOptions{Fetcher: fetcher})

	body, _ := json.Marshal(MetadataRequest{URL: "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/metadata", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp MetadataResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Description != "Desc" || resp.Title != "Title" || resp.Handle != "@example" || resp.ChannelID != "UC999" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if fetcher.url != "https://example.com" {
		t.Fatalf("expected fetcher to receive url")
	}
}

func TestMetadataHandlerErrors(t *testing.T) {
	handler := NewMetadataHandler(MetadataHandlerOptions{Fetcher: &stubMetadataFetcher{}})

	t.Run("method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rr.Code)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not json"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		fetcher := &stubMetadataFetcher{err: fmt.Errorf("%w: url required", youtubeservice.ErrValidation)}
		handler := NewMetadataHandler(MetadataHandlerOptions{Fetcher: fetcher})
		body, _ := json.Marshal(MetadataRequest{URL: ""})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("upstream error", func(t *testing.T) {
		fetcher := &stubMetadataFetcher{err: fmt.Errorf("%w: fetch failed", youtubeservice.ErrUpstream)}
		handler := NewMetadataHandler(MetadataHandlerOptions{Fetcher: fetcher})
		body, _ := json.Marshal(MetadataRequest{URL: "https://example.com"})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rr.Code)
		}
	})
}
