package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnsubscribeHandlerRequiresPost(t *testing.T) {
	handler := NewUnsubscribeHandler(UnsubscribeHandlerOptions{})
	req := httptest.NewRequest(http.MethodGet, "/api/youtube/unsubscribe", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestUnsubscribeHandlerValidatesJSON(t *testing.T) {
	handler := NewUnsubscribeHandler(UnsubscribeHandlerOptions{})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/unsubscribe", bytes.NewBufferString("{"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestUnsubscribeHandlerSuccess(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer hub.Close()

	handler := NewUnsubscribeHandler(UnsubscribeHandlerOptions{HubURL: hub.URL, Client: hub.Client()})
	payload, _ := json.Marshal(map[string]any{"topic": "https://example"})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/unsubscribe", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("expected body to match hub response")
	}
}

func TestUnsubscribeHandlerPropagatesErrors(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer hub.Close()

	handler := NewUnsubscribeHandler(UnsubscribeHandlerOptions{HubURL: hub.URL, Client: hub.Client()})
	payload, _ := json.Marshal(map[string]any{"topic": "https://example"})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/unsubscribe", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if rr.Body.String() != "bad" {
		t.Fatalf("expected body from hub")
	}
}
