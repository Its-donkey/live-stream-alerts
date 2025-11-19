package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

type stubSubscriptionProxy struct {
	result youtubeservice.SubscriptionResult
	err    error
	req    subscriptions.YouTubeRequest
}

func (s *stubSubscriptionProxy) Process(ctx context.Context, req subscriptions.YouTubeRequest) (youtubeservice.SubscriptionResult, error) {
	s.req = req
	return s.result, s.err
}

func TestSubscriptionHandlerReturnsBadRequestForValidationErrors(t *testing.T) {
	proxy := &stubSubscriptionProxy{}
	handler := NewUnsubscribeHandler(SubscriptionHandlerOptions{Proxy: proxy})
	body, _ := json.Marshal(map[string]string{
		"topic": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/unsubscribe", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for validation error, got %d", rr.Code)
	}
}

func TestSubscriptionHandlerReturnsBadGatewayForHubFailures(t *testing.T) {
	proxy := &stubSubscriptionProxy{
		err: &youtubeservice.ProxyError{Status: http.StatusBadGateway, Err: errors.New("timeout")},
	}
	handler := NewSubscribeHandler(SubscriptionHandlerOptions{Proxy: proxy})
	body, _ := json.Marshal(map[string]string{
		"topic": "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/subscribe", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for hub failure, got %d", rr.Code)
	}
}

func TestSubscriptionHandlerSuccess(t *testing.T) {
	proxy := &stubSubscriptionProxy{
		result: youtubeservice.SubscriptionResult{
			StatusCode:  http.StatusAccepted,
			ContentType: "text/plain",
			Body:        []byte("accepted"),
		},
	}
	handler := NewSubscribeHandler(SubscriptionHandlerOptions{Proxy: proxy})
	body, _ := json.Marshal(map[string]string{
		"topic": "https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/youtube/subscribe", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("expected content type to propagate")
	}
	if proxy.req.Topic == "" {
		t.Fatalf("expected proxy to receive request")
	}
}
