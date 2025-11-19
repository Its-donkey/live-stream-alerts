package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
	streamersvc "live-stream-alerts/internal/streamers/service"
)

// StreamerService describes the dependencies required by the HTTP handlers.
type StreamerService interface {
	List(ctx context.Context) ([]streamers.Record, error)
	Create(ctx context.Context, req streamersvc.CreateRequest) (streamersvc.CreateResult, error)
	Update(ctx context.Context, req streamersvc.UpdateRequest) (streamers.Record, error)
	Delete(ctx context.Context, req streamersvc.DeleteRequest) error
}

// StreamOptions configures the streamer handler.
type StreamOptions struct {
	Service StreamerService
	Logger  logging.Logger
}

type streamersHTTPHandler struct {
	service StreamerService
	logger  logging.Logger
}

// StreamersHandler returns a handler for GET/POST /api/streamers.
func StreamersHandler(opts StreamOptions) http.Handler {
	if opts.Service == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "streamer service not configured", http.StatusInternalServerError)
		})
	}
	h := &streamersHTTPHandler{service: opts.Service, logger: opts.Logger}
	return http.HandlerFunc(h.serveHTTP)
}

func (h *streamersHTTPHandler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	case http.MethodPatch:
		h.handleUpdate(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		w.Header().Set("Allow", fmt.Sprintf("%s, %s, %s, %s", http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *streamersHTTPHandler) respondError(w http.ResponseWriter, err error, defaultMessage string) {
	switch {
	case errors.Is(err, streamersvc.ErrValidation):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, streamers.ErrDuplicateAlias):
		http.Error(w, "a streamer with that alias already exists", http.StatusConflict)
	case errors.Is(err, streamers.ErrStreamerNotFound):
		http.Error(w, "streamer not found", http.StatusNotFound)
	case errors.Is(err, streamersvc.ErrSubscription):
		http.Error(w, "failed to update YouTube subscription", http.StatusBadGateway)
	default:
		if h.logger != nil {
			h.logger.Printf("%s: %v", defaultMessage, err)
		}
		http.Error(w, defaultMessage, http.StatusInternalServerError)
	}
}
