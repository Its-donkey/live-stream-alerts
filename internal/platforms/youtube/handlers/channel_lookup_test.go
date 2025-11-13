package handlers

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubTransport struct{}

func (stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"channelId":"UC1234567890123456789012"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(bytes.NewBufferString(body)),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "text/html")
	return resp, nil
}

func TestChannelLookupHandler(t *testing.T) {
	client := &http.Client{Transport: stubTransport{}}
	handler := NewChannelLookupHandler(client)

	t.Run("method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/youtube/channel", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rr.Code)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewBufferString("{"))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("missing handle", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{})
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewReader(payload))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{"handle": "@example"})
		req := httptest.NewRequest(http.MethodPost, "/api/youtube/channel", bytes.NewReader(payload))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if !bytes.Contains(rr.Body.Bytes(), []byte("UC12345678901234567890")) {
			t.Fatalf("expected response to contain channel ID")
		}
	})
}
