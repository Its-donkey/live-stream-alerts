package adminhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	adminhttp "live-stream-alerts/internal/admin/http"
	adminservice "live-stream-alerts/internal/admin/service"
)

func TestMonitorHandlerSuccess(t *testing.T) {
	svc := &stubMonitorService{
		overview: adminservice.YouTubeMonitorOverview{
			Summary: adminservice.YouTubeMonitorSummary{Total: 1},
		},
	}
	handler := adminhttp.NewMonitorHandler(adminhttp.MonitorHandlerOptions{
		Authorizer: &stubAuthorizer{},
		Service:    svc,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/monitor/youtube", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp adminservice.YouTubeMonitorOverview
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Summary.Total != 1 {
		t.Fatalf("expected summary total 1, got %+v", resp.Summary)
	}
	if !svc.called {
		t.Fatalf("expected service to be called")
	}
}

func TestMonitorHandlerUnauthorized(t *testing.T) {
	handler := adminhttp.NewMonitorHandler(adminhttp.MonitorHandlerOptions{
		Authorizer: &stubAuthorizer{err: errors.New("denied")},
		Service:    &stubMonitorService{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/monitor/youtube", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMonitorHandlerMethodNotAllowed(t *testing.T) {
	handler := adminhttp.NewMonitorHandler(adminhttp.MonitorHandlerOptions{
		Authorizer: &stubAuthorizer{},
		Service:    &stubMonitorService{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/monitor/youtube", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestMonitorHandlerServiceError(t *testing.T) {
	handler := adminhttp.NewMonitorHandler(adminhttp.MonitorHandlerOptions{
		Authorizer: &stubAuthorizer{},
		Service: &stubMonitorService{
			err: errors.New("boom"),
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/monitor/youtube", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

type stubMonitorService struct {
	overview adminservice.YouTubeMonitorOverview
	err      error
	called   bool
}

func (s *stubMonitorService) Overview(context.Context) (adminservice.YouTubeMonitorOverview, error) {
	s.called = true
	return s.overview, s.err
}
