package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
)

type channelLookupRequest struct {
	Handle string `json:"handle"`
}

type channelResolver interface {
	ResolveHandle(ctx context.Context, handle string) (string, error)
}

// ChannelLookupHandlerOptions configures the channel lookup handler.
type ChannelLookupHandlerOptions struct {
	Resolver channelResolver
	Client   *http.Client
	Logger   logging.Logger
}

type channelLookupHandler struct {
	resolver channelResolver
	logger   logging.Logger
}

// NewChannelLookupHandler resolves a YouTube handle to its canonical channel ID.
func NewChannelLookupHandler(opts ChannelLookupHandlerOptions) http.Handler {
	resolver := opts.Resolver
	if resolver == nil {
		resolver = youtubeservice.ChannelResolver{Client: opts.Client}
	}
	h := channelLookupHandler{resolver: resolver, logger: opts.Logger}
	return http.HandlerFunc(h.ServeHTTP)
}

func (h channelLookupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isPostRequest(r) {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	defer r.Body.Close()

	payload, err := decodeChannelLookupRequest(r)
	if err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	channelID, err := h.resolver.ResolveHandle(r.Context(), payload.Handle)
	if err != nil {
		h.respondError(w, err)
		return
	}

	writeChannelLookupResponse(w, payload.Handle, channelID)
}

func (h channelLookupHandler) respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, youtubeservice.ErrValidation):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, youtubeservice.ErrUpstream):
		http.Error(w, "failed to resolve channel handle", http.StatusBadGateway)
	default:
		if h.logger != nil {
			h.logger.Printf("channel lookup failed: %v", err)
		}
		http.Error(w, "failed to resolve channel handle", http.StatusInternalServerError)
	}
}

func isPostRequest(r *http.Request) bool {
	return r.Method == http.MethodPost
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func decodeChannelLookupRequest(r *http.Request) (channelLookupRequest, error) {
	var payload channelLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return payload, err
	}

	return payload, nil
}

func writeChannelLookupResponse(w http.ResponseWriter, handle, channelID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"handle":    handle,
		"channelId": channelID,
	})
}
