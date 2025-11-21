package adminhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	adminhttp "live-stream-alerts/internal/admin/http"
	"live-stream-alerts/internal/platforms/youtube/monitoring"
)

func TestMonitorHandlerSuccess(t *testing.T) {
	svc := &stubMonitorService{
		overview: monitoring.Overview{
			Summary: monitoring.Summary{Total: 1},
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
	var resp monitoring.Overview
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
	overview monitoring.Overview
	err      error
	called   bool
}

func (s *stubMonitorService) Overview(context.Context) (monitoring.Overview, error) {
	s.called = true
	return s.overview, s.err
}
