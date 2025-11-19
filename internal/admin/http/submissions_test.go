package adminhttp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"live-stream-alerts/config"
	adminauth "live-stream-alerts/internal/admin/auth"
	adminhttp "live-stream-alerts/internal/admin/http"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

func TestSubmissionsHandlerList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "submissions.json")
	data := submissions.File{Submissions: []submissions.Submission{
		{ID: "1", Alias: "Test", SubmittedAt: time.Now().UTC()},
	}}
	writeSubmissionsFile(t, path, data)

	mgr := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"})
	token, _ := mgr.Login("admin@example.com", "secret")

	handler := adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
		Manager:       mgr,
		FilePath:      path,
		StreamersPath: filepath.Join(dir, "streamers.json"),
		YouTube:       testAdminYouTubeConfig(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/submissions", nil)
	req.Header.Set("Authorization", "Bearer "+token.Value)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string][]submissions.Submission
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp["submissions"]) != 1 {
		t.Fatalf("expected one submission, got %d", len(resp["submissions"]))
	}
}

func TestSubmissionsHandlerApprove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "submissions.json")
	data := submissions.File{Submissions: []submissions.Submission{
		{ID: "1", Alias: "Test", SubmittedAt: time.Now().UTC()},
	}}
	writeSubmissionsFile(t, path, data)

	mgr := adminauth.NewManager(adminauth.Config{Email: "admin@example.com", Password: "secret"})
	token, _ := mgr.Login("admin@example.com", "secret")
	streamersPath := filepath.Join(dir, "streamers.json")
	handler := adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
		Manager:       mgr,
		FilePath:      path,
		StreamersPath: streamersPath,
		YouTube:       testAdminYouTubeConfig(),
	})

	body, _ := json.Marshal(map[string]string{"action": "approve", "id": "1"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/submissions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token.Value)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"].(string) != "approved" {
		t.Fatalf("expected status approved, got %v", resp["status"])
	}

	remaining, err := submissions.List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected submissions cleared, got %d", len(remaining))
	}

	records, err := streamers.List(streamersPath)
	if err != nil {
		t.Fatalf("list streamers: %v", err)
	}
	if len(records) != 1 || records[0].Streamer.Alias != "Test" {
		t.Fatalf("expected streamer appended on approval, got %+v", records)
	}
}

func writeSubmissionsFile(t *testing.T, path string, file submissions.File) {
	t.Helper()
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func testAdminYouTubeConfig() config.YouTubeConfig {
	return config.YouTubeConfig{
		HubURL:       "https://hub.example.com",
		CallbackURL:  "https://callback.example.com/alerts",
		Verify:       "async",
		LeaseSeconds: 60,
	}
}
