package v1

import (
	"encoding/json"
	"net/http"
	"strings"

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
	DataPath    string `json:"dataPath"`
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

	streamersHandler := streamershandlers.StreamersHandler(streamershandlers.StreamOptions{
		Logger:   logger,
		FilePath: streamersPath,
	})

	mux.Handle("/api/youtube/channel", youtubehandlers.NewChannelLookupHandler(nil))
	mux.Handle("/api/youtube/metadata", youtubehandlers.NewMetadataHandler(youtubehandlers.MetadataHandlerOptions{}))
	mux.Handle("/api/youtube/subscribe", youtubehandlers.NewSubscribeHandler(youtubehandlers.SubscribeHandlerOptions{
		Logger: logger,
	}))
	mux.Handle("/api/youtube/unsubscribe", youtubehandlers.NewUnsubscribeHandler(youtubehandlers.UnsubscribeHandlerOptions{
		Logger: logger,
	}))
	mux.Handle("/api/streamers", streamersHandler)

	mux.Handle("/alerts", handleAlerts(logger, streamersPath))

	mux.HandleFunc("/api/server/config", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, opts.RuntimeInfo)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("UI assets not configured"))
	})

	return logging.WithHTTPLogging(mux, logger)
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// handleAlerts returns an HTTP handler that only treats likely Google/YouTube
// verification requests as WebSub subscription confirmations.
func handleAlerts(logger logging.Logger, streamersPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Basic method and path sanity
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.URL.Path != "/alerts" {
			http.NotFound(w, r)
			return
		}

		// Extract headers used to judge whether this looks like Google/YouTube.
		userAgent := r.Header.Get("User-Agent")
		from := r.Header.Get("From")
		forwardedFor := r.Header.Get("X-Forwarded-For")

		platform := alertPlatform(userAgent, from)

		if platform == "youtube" {
			// Let the YouTube subscription confirmation handler do the real work:
			// validate verify_token, challenge, topic, etc.
			if youtubehandlers.HandleSubscriptionConfirmation(w, r, youtubehandlers.SubscriptionConfirmationOptions{
				Logger:        logger,
				StreamersPath: streamersPath,
			}) {
				return
			}

			// If it got this far, the request looked like Google but didn't pass
			// your internal validation (e.g. bad verify_token).
			http.Error(w, "invalid subscription confirmation", http.StatusBadRequest)
			return
		}

		// Anything that doesn't look like a genuine Google/YouTube verification
		// falls through to here.
		logger.Printf("suspicious /alerts request: platform=%q ua=%q from=%q xff=%q", platform, userAgent, from, forwardedFor)

		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
}

func alertPlatform(userAgent, from string) string {
	if strings.HasPrefix(userAgent, "FeedFetcher-Google") && from == "googlebot(at)googlebot.com" {
		return "youtube"
	}
	return ""
}


