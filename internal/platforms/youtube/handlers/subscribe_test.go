package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubscribeHandlerRequiresPost(t *testing.T) {
	handler := NewSubscribeHandler(SubscribeHandlerOptions{})
	req := httptest.NewRequest(http.MethodGet, "/api/youtube/subscribe", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestSubscribeHandlerValidatesJSON(t *testing.T) {
	handler := NewSubscribeHandler(SubscribeHandlerOptions{})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/subscribe", bytes.NewBufferString("{"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestSubscribeHandlerSuccess(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))
	defer hub.Close()

	handler := NewSubscribeHandler(SubscribeHandlerOptions{HubURL: hub.URL, Client: hub.Client()})
	payload, _ := json.Marshal(map[string]any{"topic": "https://example"})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/subscribe", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("expected body to match hub response")
	}
}

func TestSubscribeHandlerPropagatesErrors(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer hub.Close()

	handler := NewSubscribeHandler(SubscribeHandlerOptions{HubURL: hub.URL, Client: hub.Client()})
	payload, _ := json.Marshal(map[string]any{"topic": "https://example"})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/subscribe", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if rr.Body.String() != "bad" {
		t.Fatalf("expected body from hub")
	}
}
