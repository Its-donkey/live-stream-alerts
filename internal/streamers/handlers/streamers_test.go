package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

func TestStreamersHandlerGetListsStreamers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	submissionsPath := filepath.Join(dir, "submissions.json")
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

	handler := StreamersHandler(StreamOptions{FilePath: path, SubmissionsPath: submissionsPath})
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
	dir := t.TempDir()
	handler := StreamersHandler(StreamOptions{
		FilePath:        filepath.Join(dir, "streamers.json"),
		SubmissionsPath: filepath.Join(dir, "submissions.json"),
	})
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
	submissionsPath := filepath.Join(dir, "submissions.json")
	handler := StreamersHandler(StreamOptions{FilePath: path, SubmissionsPath: submissionsPath})

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

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "pending" {
		t.Fatalf("expected pending status, got %q", resp["status"])
	}

	pending, err := submissions.List(submissionsPath)
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(pending) != 1 || pending[0].Alias != "Test Alias" {
		t.Fatalf("expected pending submission, got %+v", pending)
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no streamers persisted yet, got %d", len(records))
	}
}

func TestStreamersHandlerPostDuplicateAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	submissionsPath := filepath.Join(dir, "submissions.json")
	handler := StreamersHandler(StreamOptions{FilePath: path, SubmissionsPath: submissionsPath})

	first := map[string]any{
		"streamer": map[string]any{
			"alias": "Edge Crafter",
		},
		"platforms": map[string]any{},
	}
	second := map[string]any{
		"streamer": map[string]any{
			"alias": "EdgeCrafter!!!",
		},
		"platforms": map[string]any{},
	}

	for idx, payload := range []map[string]any{first, second} {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if idx == 0 && rr.Code != http.StatusAccepted {
			t.Fatalf("expected first submission accepted, got %d", rr.Code)
		}
		if idx == 1 && rr.Code != http.StatusConflict {
			t.Fatalf("expected duplicate alias to return 409, got %d", rr.Code)
		}
	}
	pending, err := submissions.List(submissionsPath)
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected single pending submission, got %d", len(pending))
	}
}

func TestStreamersHandlerDeleteSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	var unsubCalled bool
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("hub.mode"); got != "unsubscribe" {
			t.Fatalf("expected hub.mode to be unsubscribe, got %s", got)
		}
		unsubCalled = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()

	record, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{
			ID:        "ToDelete",
			Alias:     "ToDelete",
			FirstName: "Delete",
			LastName:  "Me",
			Email:     "delete@example.com",
		},
		Platforms: streamers.Platforms{
			YouTube: &streamers.YouTubePlatform{
				ChannelID:   "UC555",
				HubSecret:   "secret",
				CallbackURL: "https://callback.example.com/alerts",
			},
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	handler := StreamersHandler(StreamOptions{
		FilePath:        path,
		SubmissionsPath: filepath.Join(dir, "submissions.json"),
		YouTubeClient:   hub.Client(),
		YouTubeHubURL:   hub.URL,
	})
	payload := map[string]any{
		"streamer": map[string]string{
			"id": record.Streamer.ID,
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
	if !unsubCalled {
		t.Fatalf("expected unsubscribe to be called before deletion")
	}
}

func TestStreamersHandlerDeleteUnsubscribeFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer hub.Close()

	record, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{
			ID:        "FailDelete",
			Alias:     "FailDelete",
			FirstName: "Delete",
			LastName:  "Me",
			Email:     "delete@example.com",
		},
		Platforms: streamers.Platforms{
			YouTube: &streamers.YouTubePlatform{
				ChannelID:   "UCfail",
				HubSecret:   "secret",
				CallbackURL: "https://callback.example.com/alerts",
			},
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	handler := StreamersHandler(StreamOptions{
		FilePath:        path,
		SubmissionsPath: filepath.Join(dir, "submissions.json"),
		YouTubeClient:   hub.Client(),
		YouTubeHubURL:   hub.URL,
	})
	body, _ := json.Marshal(map[string]any{
		"streamer": map[string]any{
			"id": record.Streamer.ID,
		},
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/streamers/"+record.Streamer.ID, bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when unsubscribe fails, got %d", rr.Code)
	}

	records, err := streamers.List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected record to remain when unsubscribe fails, found %d", len(records))
	}
}

func TestStreamersHandlerDeleteValidations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	handler := StreamersHandler(StreamOptions{FilePath: path, SubmissionsPath: filepath.Join(dir, "submissions.json")})

	t.Run("missing id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/streamers", bytes.NewBufferString(`{"streamer":{"id":""}}`))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing id, got %d", rr.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/streamers", bytes.NewBufferString(`{"streamer":{"id":"missing"}}`))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing streamer, got %d", rr.Code)
		}
	})
}

func TestStreamersHandlerPatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	record, err := streamers.Append(path, streamers.Record{
		Streamer: streamers.Streamer{
			ID:        "PatchMe",
			Alias:     "PatchMe",
			FirstName: "Pat",
			LastName:  "Chable",
			Email:     "patch@example.com",
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	handler := StreamersHandler(StreamOptions{FilePath: path, SubmissionsPath: filepath.Join(dir, "submissions.json")})

	t.Run("success", func(t *testing.T) {
		payload := map[string]any{
			"streamer": map[string]any{
				"id":          record.Streamer.ID,
				"alias":       "New Alias",
				"description": "Updated description",
				"languages":   []string{"English"},
			},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPatch, "/api/streamers", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !bytes.Contains(rr.Body.Bytes(), []byte("New Alias")) {
			t.Fatalf("expected updated alias in response")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		payload := map[string]any{
			"streamer": map[string]any{
				"id": record.Streamer.ID,
			},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPatch, "/api/streamers", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for missing fields, got %d", rr.Code)
		}
	})
}

func TestStreamersHandlerMethodNotAllowed(t *testing.T) {
	dir := t.TempDir()
	handler := StreamersHandler(StreamOptions{SubmissionsPath: filepath.Join(dir, "submissions.json"), FilePath: filepath.Join(dir, "streamers.json")})
	req := httptest.NewRequest(http.MethodPut, "/api/streamers", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != "GET, POST, PATCH, DELETE" {
		t.Fatalf("expected Allow header to advertise supported methods, got %q", allow)
	}
}
