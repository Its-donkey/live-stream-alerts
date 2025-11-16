package v1

import (
	"encoding/json"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/liveinfo"
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
	YouTubeAPIKey string
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

	var videoLookup youtubehandlers.LiveVideoLookup
	if key := strings.TrimSpace(opts.YouTubeAPIKey); key != "" {
		videoLookup = &liveinfo.Client{APIKey: key}
	}

	alertsHandler := handleAlerts(handleAlertsOptions{
		Logger:        logger,
		StreamersPath: streamersPath,
		VideoLookup:   videoLookup,
	})
	mux.Handle("/alerts", alertsHandler)
	mux.Handle("/alert", alertsHandler)

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

type handleAlertsOptions struct {
	Logger        logging.Logger
	StreamersPath string
	VideoLookup   youtubehandlers.LiveVideoLookup
}

func handleAlerts(opts handleAlertsOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if youtubehandlers.HandleSubscriptionConfirmation(w, r, youtubehandlers.SubscriptionConfirmationOptions{
			Logger:        opts.Logger,
			StreamersPath: opts.StreamersPath,
		}) {
			return
		}
		if youtubehandlers.HandleAlertNotification(w, r, youtubehandlers.AlertNotificationOptions{
			Logger:        opts.Logger,
			StreamersPath: opts.StreamersPath,
			VideoLookup:   opts.VideoLookup,
		}) {
			return
		}
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
}
