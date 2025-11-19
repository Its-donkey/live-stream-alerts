package adminhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminhttp "live-stream-alerts/internal/admin/http"
	adminservice "live-stream-alerts/internal/admin/service"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

func TestSubmissionsHandlerList(t *testing.T) {
	svc := &stubSubmissionsService{
		list: []submissions.Submission{{ID: "1", Alias: "Test", SubmittedAt: time.Now()}},
	}
	handler := adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
		Authorizer: &stubAuthorizer{},
		Service:    svc,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/submissions", nil)
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
		t.Fatalf("expected 1 submission, got %d", len(resp["submissions"]))
	}
}

func TestSubmissionsHandlerUnauthorized(t *testing.T) {
	handler := adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
		Authorizer: &stubAuthorizer{err: errors.New("nope")},
		Service:    &stubSubmissionsService{},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/submissions", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestSubmissionsHandlerApprove(t *testing.T) {
	svc := &stubSubmissionsService{
		result: adminservice.ActionResult{
			Status:     adminservice.ActionApprove,
			Submission: submissions.Submission{ID: "1"},
		},
	}
	handler := adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
		Authorizer: &stubAuthorizer{},
		Service:    svc,
	})
	body, _ := json.Marshal(map[string]string{"action": "approve", "id": "1"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/submissions", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if svc.received.Action != adminservice.ActionApprove || svc.received.ID != "1" {
		t.Fatalf("service received wrong request: %+v", svc.received)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != string(adminservice.ActionApprove) {
		t.Fatalf("expected approve status, got %v", resp["status"])
	}
}

func TestSubmissionsHandlerProcessErrors(t *testing.T) {
	cases := []struct {
		err            error
		expectedStatus int
	}{
		{adminservice.ErrInvalidAction, http.StatusBadRequest},
		{adminservice.ErrMissingIdentifier, http.StatusBadRequest},
		{submissions.ErrNotFound, http.StatusNotFound},
		{streamers.ErrDuplicateAlias, http.StatusConflict},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			handler := adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
				Authorizer: &stubAuthorizer{},
				Service: &stubSubmissionsService{
					processErr: tc.err,
				},
			})
			body, _ := json.Marshal(map[string]string{"action": "approve", "id": "1"})
			req := httptest.NewRequest(http.MethodPost, "/api/admin/submissions", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			if rr.Code != tc.expectedStatus {
				t.Fatalf("expected %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}

type stubAuthorizer struct {
	err error
}

func (s *stubAuthorizer) AuthorizeRequest(*http.Request) error {
	return s.err
}

type stubSubmissionsService struct {
	list       []submissions.Submission
	listErr    error
	result     adminservice.ActionResult
	processErr error
	received   adminservice.ActionRequest
}

func (s *stubSubmissionsService) List(context.Context) ([]submissions.Submission, error) {
	return s.list, s.listErr
}

func (s *stubSubmissionsService) Process(ctx context.Context, req adminservice.ActionRequest) (adminservice.ActionResult, error) {
	s.received = req
	return s.result, s.processErr
}
