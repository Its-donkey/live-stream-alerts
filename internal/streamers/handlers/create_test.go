package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"live-stream-alerts/internal/streamers"
)

func TestCreateHandlerGetListsStreamers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	_, err := streamers.Append(path, streamers.Record{Streamer: streamers.Streamer{Alias: "One", FirstName: "One", LastName: "User", Email: "one@example.com"}})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	handler := NewCreateHandler(CreateOptions{FilePath: path})
	req := httptest.NewRequest(http.MethodGet, "/api/streamers", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("One")) {
		t.Fatalf("expected response to contain streamer alias")
	}
}

func TestCreateHandlerPostValidation(t *testing.T) {
	handler := NewCreateHandler(CreateOptions{FilePath: filepath.Join(t.TempDir(), "streamers.json")})
	req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", rr.Code)
	}
}

func TestCreateHandlerPostSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	handler := NewCreateHandler(CreateOptions{FilePath: path, YouTubeClient: hub.Client(), YouTubeHubURL: hub.URL})

	record := streamers.Record{
		Streamer: streamers.Streamer{
			Alias:     "Test123",
			FirstName: "Test",
			LastName:  "User",
			Email:     "test@example.com",
			Languages: []string{"English", "Japanese"},
		},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{Handle: "@test", ChannelID: "UC12345678901234567890"}},
	}
	payload, _ := json.Marshal(record)
	req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("Test123")) {
		t.Fatalf("expected response to include alias")
	}
}

func TestCreateHandlerMethodNotAllowed(t *testing.T) {
	handler := NewCreateHandler(CreateOptions{})
	req := httptest.NewRequest(http.MethodDelete, "/api/streamers", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}
