package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDescriptionHandlerSuccess(t *testing.T) {
	html := `<!doctype html><html><head><meta name="description" content="Desc"><meta property="og:title" content="Title"><meta property="og:url" content="https://www.youtube.com/@example"><meta itemprop="channelId" content="UC999"></head></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(server.Close)

	handler := NewDescriptionHandler(DescriptionHandlerOptions{Client: server.Client()})

	body, _ := json.Marshal(DescriptionRequest{URL: server.URL})
	req := httptest.NewRequest(http.MethodPost, "/api/metadata/description", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp DescriptionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Description != "Desc" || resp.Title != "Title" || resp.Handle != "@example" || resp.ChannelID != "UC999" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestNewDescriptionHandlerValidatesInput(t *testing.T) {
	handler := NewDescriptionHandler(DescriptionHandlerOptions{})

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

	t.Run("invalid URL", func(t *testing.T) {
		payload, _ := json.Marshal(DescriptionRequest{URL: "ftp://example"})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})
}

func TestFetchDescriptionParsesContent(t *testing.T) {
	html := `<!doctype html><html><head><title>Alt</title><meta property="og:url" content="https://youtube.com/@other"><link rel="canonical" href="https://youtube.com/channel/UC111"><meta itemprop="channelId" content="UC111"></head></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	desc, title, handle, channelID, err := fetchDescription(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Alt" {
		t.Fatalf("expected title, got %q", title)
	}
	if handle != "@other" {
		t.Fatalf("expected handle, got %q", handle)
	}
	if channelID != "UC111" {
		t.Fatalf("expected channel ID, got %q", channelID)
	}
	if desc != "Alt" {
		t.Fatalf("expected desc fallback to title")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", " value ", "another"); got != "value" {
		t.Fatalf("expected trimmed first non-empty, got %q", got)
	}
	if firstNonEmpty() != "" {
		t.Fatalf("expected empty when no values")
	}
}

func TestDeriveHandle(t *testing.T) {
	if deriveHandle("https://youtube.com/@handle") != "@handle" {
		t.Fatalf("expected handle to be returned")
	}
	if deriveHandle("https://youtube.com/channel/UC123") != "" {
		t.Fatalf("expected empty handle")
	}
}

func TestParseChannelID(t *testing.T) {
	if parseChannelID("https://youtube.com/channel/UC999") != "UC999" {
		t.Fatalf("expected channel ID")
	}
	if parseChannelID("https://youtube.com/user/test") != "" {
		t.Fatalf("expected empty when pattern missing")
	}
}
