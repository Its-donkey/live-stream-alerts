package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

// StreamOptions configures the streamer handler.
type StreamOptions struct {
	FilePath      string
	Logger        logging.Logger
	YouTubeClient *http.Client
	YouTubeHubURL string
}

// StreamersHandler returns a handler for GET/POST /api/streamers.
func StreamersHandler(opts StreamOptions) http.Handler {
	path := opts.FilePath
	if path == "" {
		path = streamers.DefaultFilePath
	}
	path = filepath.Clean(path)

	youtubeClient := opts.YouTubeClient
	if youtubeClient == nil {
		youtubeClient = &http.Client{Timeout: 10 * time.Second}
	}
	youtubeHubURL := strings.TrimSpace(opts.YouTubeHubURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listStreamers(w, path, opts.Logger)
			return
		case http.MethodPost:
			createStreamer(w, r, path, opts.Logger, youtubeClient, youtubeHubURL)
			return
		case http.MethodPatch:
			updateStreamer(w, r, path, opts.Logger)
			return
		case http.MethodDelete:
			deleteStreamer(w, r, path, opts.Logger, youtubeClient, youtubeHubURL)
			return
		default:
			w.Header().Set("Allow", fmt.Sprintf("%s, %s, %s, %s", http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})
}
