package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"live-stream-alerts/internal/streamers"
)

func TestStreamersHandlerGetListsStreamers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	for _, alias := range []string{"One", "Two"} {
		if _, err := streamers.Append(path, streamers.Record{
			Streamer: streamers.Streamer{
				Alias:     alias,
				FirstName: alias,
				LastName:  "User",
				Email:     alias + "@example.com",
			},
		}); err != nil {
			t.Fatalf("append %s: %v", alias, err)
		}
	}

	handler := StreamersHandler(StreamOptions{FilePath: path})
	req := httptest.NewRequest(http.MethodGet, "/api/streamers", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Streamers []streamers.Record `json:"streamers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Streamers) != 2 {
		t.Fatalf("expected 2 streamers, got %d", len(resp.Streamers))
	}
}

func TestStreamersHandlerPostValidation(t *testing.T) {
	handler := StreamersHandler(StreamOptions{FilePath: filepath.Join(t.TempDir(), "streamers.json")})
	req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid json, got %d", rr.Code)
	}
}

func TestStreamersHandlerPostSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	handler := StreamersHandler(StreamOptions{FilePath: path})

	payload := map[string]any{
		"streamer": map[string]any{
			"alias":       "Test Alias",
			"description": "Example description",
			"languages":   []string{"English", "English", "Japanese"},
		},
		"platforms": map[string]any{
			"url": "",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var record streamers.Record
	if err := json.Unmarshal(rr.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if record.Streamer.Alias != "Test Alias" {
		t.Fatalf("expected alias to be stored, got %q", record.Streamer.Alias)
	}
	if record.Streamer.ID != "TestAlias" {
		t.Fatalf("expected ID to be derived from alias, got %q", record.Streamer.ID)
	}
	if len(record.Streamer.Languages) != 2 {
		t.Fatalf("expected duplicate languages removed, got %v", record.Streamer.Languages)
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record persisted, got %d", len(records))
	}
}

func TestStreamersHandlerDeleteSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	record, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{
			ID:        "ToDelete",
			Alias:     "ToDelete",
			FirstName: "Delete",
			LastName:  "Me",
			Email:     "delete@example.com",
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	handler := StreamersHandler(StreamOptions{FilePath: path})
	payload := map[string]any{
		"streamer": map[string]string{
			"id":        record.Streamer.ID,
			"createdAt": record.CreatedAt.Format(time.RFC3339Nano),
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodDelete, "/api/streamers/"+record.Streamer.ID, bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"status":"deleted"`)) {
		t.Fatalf("expected delete confirmation payload")
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected record to be deleted, still have %d", len(records))
	}
}

func TestStreamersHandlerDeleteValidations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	handler := StreamersHandler(StreamOptions{FilePath: path})

	t.Run("missing path id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/streamers/", bytes.NewBufferString(`{"streamer":{"id":"","createdAt":""}}`))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing path parameter, got %d", rr.Code)
		}
	})

	t.Run("body mismatch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/streamers/one", bytes.NewBufferString(`{"streamer":{"id":"two","createdAt":"2025-01-01T00:00:00Z"}}`))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for id mismatch, got %d", rr.Code)
		}
	})

	t.Run("timestamp mismatch", func(t *testing.T) {
		rec, err := streamers.Append(path, streamers.Record{
			Streamer: streamers.Streamer{
				ID:        "HasTimestamp",
				Alias:     "HasTimestamp",
				FirstName: "Time",
				LastName:  "Stamp",
				Email:     "time@example.com",
			},
		})
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		defer func() {
			_ = streamers.Delete(path, rec.Streamer.ID, rec.CreatedAt.Format(time.RFC3339Nano))
		}()

		req := httptest.NewRequest(
			http.MethodDelete,
			"/api/streamers/"+rec.Streamer.ID,
			bytes.NewBufferString(`{"streamer":{"id":"`+rec.Streamer.ID+`","createdAt":"`+rec.CreatedAt.Add(time.Hour).Format(time.RFC3339Nano)+`"}}`),
		)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409 for timestamp mismatch, got %d", rr.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/streamers/missing", bytes.NewBufferString(`{"streamer":{"id":"missing","createdAt":"2025-01-01T00:00:00Z"}}`))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing streamer, got %d", rr.Code)
		}
	})
}

func TestStreamersHandlerMethodNotAllowed(t *testing.T) {
	handler := StreamersHandler(StreamOptions{})
	req := httptest.NewRequest(http.MethodPut, "/api/streamers", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != "GET, POST, DELETE" {
		t.Fatalf("expected Allow header to advertise supported methods, got %q", allow)
	}
}
