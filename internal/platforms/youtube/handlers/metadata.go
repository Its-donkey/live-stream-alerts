package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"live-stream-alerts/internal/logging"
	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
)

// MetadataRequest describes the payload for fetching metadata.
type MetadataRequest struct {
	URL string `json:"url"`
}

// MetadataResponse represents the metadata information returned to the client.
type MetadataResponse struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	Handle      string `json:"handle"`
	ChannelID   string `json:"channelId"`
}

type metadataFetcher interface {
	Fetch(ctx context.Context, rawURL string) (youtubeservice.Metadata, error)
}

// MetadataHandlerOptions configures the metadata handler.
type MetadataHandlerOptions struct {
	Fetcher metadataFetcher
	Client  *http.Client
	Logger  logging.Logger
}

type metadataHandler struct {
	fetcher metadataFetcher
	logger  logging.Logger
}

// NewMetadataHandler returns an http.Handler that fetches metadata for a given URL.
func NewMetadataHandler(opts MetadataHandlerOptions) http.Handler {
	fetcher := opts.Fetcher
	if fetcher == nil {
		fetcher = youtubeservice.MetadataService{
			Client:  opts.Client,
			Timeout: 5 * time.Second,
		}
	}
	h := metadataHandler{fetcher: fetcher, logger: opts.Logger}
	return http.HandlerFunc(h.ServeHTTP)
}

func (h metadataHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isPostRequest(r) {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	defer r.Body.Close()

	req, err := decodeMetadataRequest(r)
	if err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	data, fetchErr := h.fetcher.Fetch(ctx, req.URL)
	if fetchErr != nil {
		h.respondError(w, fetchErr)
		return
	}

	writeMetadataResponse(w, MetadataResponse{
		Description: data.Description,
		Title:       data.Title,
		Handle:      data.Handle,
		ChannelID:   data.ChannelID,
	})
}

func (h metadataHandler) respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, youtubeservice.ErrValidation):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, youtubeservice.ErrUpstream):
		http.Error(w, "failed to fetch metadata", http.StatusBadGateway)
	default:
		if h.logger != nil {
			h.logger.Printf("metadata fetch failed: %v", err)
		}
		http.Error(w, "failed to fetch metadata", http.StatusInternalServerError)
	}
}

func decodeMetadataRequest(r *http.Request) (MetadataRequest, error) {
	var req MetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func writeMetadataResponse(w http.ResponseWriter, resp MetadataResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
