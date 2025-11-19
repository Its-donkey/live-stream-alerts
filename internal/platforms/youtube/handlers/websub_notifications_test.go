package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
)

type stubAlertProcessor struct {
	result youtubeservice.AlertProcessResult
	err    error
	calls  int
}

func (s *stubAlertProcessor) Process(ctx context.Context, req youtubeservice.AlertProcessRequest) (youtubeservice.AlertProcessResult, error) {
	s.calls++
	return s.result, s.err
}

func TestHandleAlertNotificationDelegatesToProcessor(t *testing.T) {
	stub := &stubAlertProcessor{
		result: youtubeservice.AlertProcessResult{
			Entries:     1,
			VideoIDs:    []string{"abc123"},
			LiveUpdates: []youtubeservice.LiveUpdate{{ChannelID: "UCdemo", VideoID: "abc123"}},
		},
	}
	opts := AlertNotificationOptions{Processor: stub}
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString("<feed/>"))
	rr := httptest.NewRecorder()

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to process request")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if stub.calls != 1 {
		t.Fatalf("expected processor to be called once, got %d", stub.calls)
	}
}

func TestHandleAlertNotificationHandlesInvalidFeedError(t *testing.T) {
	stub := &stubAlertProcessor{err: youtubeservice.ErrInvalidFeed}
	opts := AlertNotificationOptions{Processor: stub}
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString("payload"))
	rr := httptest.NewRecorder()

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to process request")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleAlertNotificationHandlesLookupError(t *testing.T) {
	stub := &stubAlertProcessor{
		err: youtubeservice.ErrLookupFailed,
		result: youtubeservice.AlertProcessResult{
			VideoIDs: []string{"abc123"},
		},
	}
	opts := AlertNotificationOptions{Processor: stub}
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString("payload"))
	rr := httptest.NewRecorder()

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to process request")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
}

func TestHandleAlertNotificationHandlesUnknownError(t *testing.T) {
	stub := &stubAlertProcessor{err: errors.New("boom")}
	opts := AlertNotificationOptions{Processor: stub}
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString("payload"))
	rr := httptest.NewRecorder()

	if !HandleAlertNotification(rr, req, opts) {
		t.Fatalf("expected handler to process request")
	}
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestHandleAlertNotificationSkipsUnsupportedPaths(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/other", nil)
	if HandleAlertNotification(httptest.NewRecorder(), req, AlertNotificationOptions{}) {
		t.Fatalf("expected handler to ignore unsupported paths")
	}
}
