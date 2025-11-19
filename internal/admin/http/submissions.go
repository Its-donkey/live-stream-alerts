package adminhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"live-stream-alerts/config"
	adminauth "live-stream-alerts/internal/admin/auth"
	adminservice "live-stream-alerts/internal/admin/service"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

// SubmissionsHandlerOptions configures the admin submissions handler.
type SubmissionsHandlerOptions struct {
	Authorizer       authorizer
	Service          submissionsService
	Manager          *adminauth.Manager
	SubmissionsStore *submissions.Store
	StreamersStore   *streamers.Store
	YouTubeClient    *http.Client
	Logger           logging.Logger
	YouTube          config.YouTubeConfig
}

type authorizer interface {
	AuthorizeRequest(*http.Request) error
}

type submissionsService interface {
	List(ctx context.Context) ([]submissions.Submission, error)
	Process(ctx context.Context, req adminservice.ActionRequest) (adminservice.ActionResult, error)
}

type submissionsHandler struct {
	authorizer authorizer
	service    submissionsService
	logger     logging.Logger
}

// NewSubmissionsHandler constructs the admin submissions HTTP handler.
func NewSubmissionsHandler(opts SubmissionsHandlerOptions) http.Handler {
	auth := opts.Authorizer
	if auth == nil && opts.Manager != nil {
		auth = adminservice.AuthService{Manager: opts.Manager}
	}
	svc := opts.Service
	if svc == nil {
		svc = adminservice.NewSubmissionsService(adminservice.SubmissionsOptions{
			SubmissionsStore: opts.SubmissionsStore,
			StreamersStore:   opts.StreamersStore,
			YouTubeClient:    opts.YouTubeClient,
			YouTube:          opts.YouTube,
			Logger:           opts.Logger,
		})
	}
	return submissionsHandler{
		authorizer: auth,
		service:    svc,
		logger:     opts.Logger,
	}
}

func (h submissionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.authorizer == nil || h.service == nil {
		http.Error(w, "admin submissions disabled", http.StatusServiceUnavailable)
		return
	}
	if err := h.authorizer.AuthorizeRequest(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.update(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h submissionsHandler) list(w http.ResponseWriter, r *http.Request) {
	pending, err := h.service.List(r.Context())
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("list submissions: %v", err)
		}
		http.Error(w, "failed to load submissions", http.StatusInternalServerError)
		return
	}
	respondJSON(w, map[string]any{"submissions": pending})
}

func (h submissionsHandler) update(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req adminservice.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	result, err := h.service.Process(r.Context(), req)
	if err != nil {
		h.handleProcessError(w, err)
		return
	}
	respondJSON(w, map[string]any{
		"status":     result.Status,
		"submission": result.Submission,
	})
}

func (h submissionsHandler) handleProcessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, adminservice.ErrInvalidAction):
		http.Error(w, "action must be approve or reject", http.StatusBadRequest)
	case errors.Is(err, adminservice.ErrMissingIdentifier):
		http.Error(w, "id is required", http.StatusBadRequest)
	case errors.Is(err, submissions.ErrNotFound):
		http.Error(w, "submission not found", http.StatusNotFound)
	case errors.Is(err, streamers.ErrDuplicateAlias):
		http.Error(w, "a streamer with that alias already exists", http.StatusConflict)
	default:
		if h.logger != nil {
			h.logger.Printf("update submission: %v", err)
		}
		http.Error(w, "failed to update submission", http.StatusInternalServerError)
	}
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}
