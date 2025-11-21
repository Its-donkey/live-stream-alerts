package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataServiceFetch(t *testing.T) {
	html := `<!doctype html><html><head><meta name="description" content="Desc"><meta property="og:title" content="Title"><meta property="og:url" content="https://www.youtube.com/@example"><meta itemprop="channelId" content="UC999"></head></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(server.Close)

	svc := MetadataService{Client: server.Client()}
	data, err := svc.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if data.Description != "Desc" || data.Title != "Title" || data.Handle != "@example" || data.ChannelID != "UC999" {
		t.Fatalf("unexpected metadata: %+v", data)
	}
}

func TestMetadataServiceValidation(t *testing.T) {
	svc := MetadataService{}
	if _, err := svc.Fetch(context.Background(), "ftp://example"); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestFetchMetadataParsesContent(t *testing.T) {
	html := `<!doctype html><html><head><title>Alt</title><meta property="og:url" content="https://youtube.com/@other"><link rel="canonical" href="https://youtube.com/channel/UC111"><meta itemprop="channelId" content="UC111"></head></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	svc := MetadataService{Client: server.Client()}
	desc, title, handle, channelID, err := svc.fetchMetadata(context.Background(), server.URL)
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
		t.Fatalf("expected desc fallback to title, got %q", desc)
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
