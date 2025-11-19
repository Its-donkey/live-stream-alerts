package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

// StreamOptions configures the streamer handler.
type StreamOptions struct {
	Store            *streamers.Store
	Logger           logging.Logger
	YouTubeClient    *http.Client
	YouTubeHubURL    string
	SubmissionsStore *submissions.Store
}

// StreamersHandler returns a handler for GET/POST /api/streamers.
func StreamersHandler(opts StreamOptions) http.Handler {
	store := opts.Store
	if store == nil {
		store = streamers.NewStore(streamers.DefaultFilePath)
	}
	submissionsStore := opts.SubmissionsStore
	if submissionsStore == nil {
		submissionsStore = submissions.NewStore(submissions.DefaultFilePath)
	}

	youtubeClient := opts.YouTubeClient
	if youtubeClient == nil {
		youtubeClient = &http.Client{Timeout: 10 * time.Second}
	}
	youtubeHubURL := strings.TrimSpace(opts.YouTubeHubURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listStreamers(w, store, opts.Logger)
			return
		case http.MethodPost:
			createStreamer(w, r, store, submissionsStore, opts.Logger)
			return
		case http.MethodPatch:
			updateStreamer(w, r, store, opts.Logger)
			return
		case http.MethodDelete:
			deleteStreamer(w, r, store, opts.Logger, youtubeClient, youtubeHubURL)
			return
		default:
			w.Header().Set("Allow", fmt.Sprintf("%s, %s, %s, %s", http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})
}
