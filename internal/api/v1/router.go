package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"

	"live-stream-alerts/internal/logging"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	streamershandlers "live-stream-alerts/internal/streamers/handlers"
)

// RuntimeInfo describes the pieces of server configuration that the UI exposes.
type RuntimeInfo struct {
	Name        string `json:"name"`
	Addr        string `json:"addr"`
	Port        string `json:"port"`
	ReadTimeout string `json:"readTimeout"`
}

// Options configures the HTTP router.
type Options struct {
	Logger        logging.Logger
	RuntimeInfo   RuntimeInfo
	StreamersPath string
}

// NewRouter constructs the HTTP router for the public API.
func NewRouter(opts Options) http.Handler {
	mux := http.NewServeMux()
	logger := opts.Logger
	streamersPath := opts.StreamersPath

	mux.Handle("/api/youtube/subscribe", youtubehandlers.NewSubscribeHandler(youtubehandlers.SubscribeHandlerOptions{
		Logger: logger,
	}))

	mux.Handle("/api/youtube/channel", youtubehandlers.NewChannelLookupHandler(nil))

	streamersHandler := streamershandlers.StreamersHandler(streamershandlers.StreamOptions{
		Logger:   logger,
		FilePath: streamersPath,
	})
	mux.Handle("/api/streamers", streamersHandler)
	mux.Handle("/api/streamers/", streamersHandler)

	mux.Handle("/api/youtube/description", youtubehandlers.NewDescriptionHandler(youtubehandlers.DescriptionHandlerOptions{}))

	mux.HandleFunc("/api/server/config", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, opts.RuntimeInfo)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("UI assets not configured"))
	})

	if logger == nil {
		return mux
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dump, err := httputil.DumpRequest(r, true); err == nil {
			logger.Printf("---- Incoming request from %s ----\n%s", r.RemoteAddr, dump)
		} else {
			logger.Printf("failed to dump request from %s: %v", r.RemoteAddr, err)
		}
		mux.ServeHTTP(w, r)
	})
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
