package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"live-stream-alerts/internal/streamers"
	streamersvc "live-stream-alerts/internal/streamers/service"
)

type fakeService struct {
	listResp   []streamers.Record
	listErr    error
	createErr  error
	updateResp streamers.Record
	updateErr  error
	deleteErr  error
	lastCreate streamersvc.CreateRequest
	lastUpdate streamersvc.UpdateRequest
	lastDelete streamersvc.DeleteRequest
}

func (f *fakeService) List(ctx context.Context) ([]streamers.Record, error) {
	return f.listResp, f.listErr
}

func (f *fakeService) Create(ctx context.Context, req streamersvc.CreateRequest) (streamersvc.CreateResult, error) {
	f.lastCreate = req
	return streamersvc.CreateResult{}, f.createErr
}

func (f *fakeService) Update(ctx context.Context, req streamersvc.UpdateRequest) (streamers.Record, error) {
	f.lastUpdate = req
	return f.updateResp, f.updateErr
}

func (f *fakeService) Delete(ctx context.Context, req streamersvc.DeleteRequest) error {
	f.lastDelete = req
	return f.deleteErr
}

func TestStreamersHandlerList(t *testing.T) {
	service := &fakeService{listResp: []streamers.Record{{Streamer: streamers.Streamer{Alias: "one"}}}}
	handler := StreamersHandler(StreamOptions{Service: service})
	req := httptest.NewRequest(http.MethodGet, "/api/streamers", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestStreamersHandlerCreateValidatesJSON(t *testing.T) {
	handler := StreamersHandler(StreamOptions{Service: &fakeService{}})
	req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewBufferString("not json"))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestStreamersHandlerCreateDuplicate(t *testing.T) {
	service := &fakeService{createErr: streamers.ErrDuplicateAlias}
	handler := StreamersHandler(StreamOptions{Service: service})
	payload := map[string]any{"streamer": map[string]any{"alias": "Test"}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/streamers", bytes.NewReader(body))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.Code)
	}
}

func TestStreamersHandlerUpdateSuccess(t *testing.T) {
	service := &fakeService{updateResp: streamers.Record{Streamer: streamers.Streamer{ID: "abc"}}}
	handler := StreamersHandler(StreamOptions{Service: service})
	payload := map[string]any{"streamer": map[string]any{"id": "abc", "alias": "New"}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPatch, "/api/streamers", bytes.NewReader(body))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if service.lastUpdate.ID != "abc" {
		t.Fatalf("expected update to propagate id")
	}
}

func TestStreamersHandlerUpdateValidationError(t *testing.T) {
	service := &fakeService{updateErr: streamersvc.ErrValidation}
	handler := StreamersHandler(StreamOptions{Service: service})
	payload := map[string]any{"streamer": map[string]any{"id": "abc"}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPatch, "/api/streamers", bytes.NewReader(body))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestStreamersHandlerDeleteMapsErrors(t *testing.T) {
	service := &fakeService{deleteErr: streamers.ErrStreamerNotFound}
	handler := StreamersHandler(StreamOptions{Service: service})
	payload := map[string]any{"streamer": map[string]any{"id": "missing"}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodDelete, "/api/streamers", bytes.NewReader(body))
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestStreamersHandlerPropagatesInternalErrors(t *testing.T) {
	service := &fakeService{listErr: errors.New("boom")}
	handler := StreamersHandler(StreamOptions{Service: service})
	req := httptest.NewRequest(http.MethodGet, "/api/streamers", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
}
