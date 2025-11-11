package v1

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httputil"

	"live-stream-alerts/internal/logging"
	metadatahandlers "live-stream-alerts/internal/metadata/handlers"
	ytclienthandlers "live-stream-alerts/internal/platforms/youtube/handlers"
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
	Logger      logging.Logger
	StaticFS    fs.FS
	RuntimeInfo RuntimeInfo
}

func New(opts Options) http.Handler {
	mux := http.NewServeMux()
	logger := opts.Logger

	mux.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		ytclienthandlers.SubscriptionConfirmation(w, r, logger)
	})

	mux.Handle("/api/v1/youtube/subscribe", ytclienthandlers.NewSubscribeHandler(ytclienthandlers.YouTubeSubscribeOptions{
		Logger: logger,
	}))

	mux.Handle("/api/v1/youtube/channel", ytclienthandlers.ChannelLookupHandler(nil))

	mux.Handle("/api/v1/streamers", streamershandlers.NewCreateHandler(streamershandlers.CreateOptions{
		Logger: logger,
	}))

	mux.Handle("/api/v1/metadata/description", metadatahandlers.DescriptionHandler(metadatahandlers.DescriptionHandlerOptions{}))

	mux.HandleFunc("/api/v1/server/config", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, opts.RuntimeInfo)
	})

	if opts.StaticFS != nil {
		fileServer := http.FileServer(http.FS(opts.StaticFS))
		mux.Handle("/", fileServer)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("alGUI assets not configured"))
		})
	}

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
