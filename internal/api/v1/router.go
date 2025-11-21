package v1

import (
	"io"
	"net/http"
	"strings"

	"live-stream-alerts/config"
	"live-stream-alerts/internal/logging"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/streamers"
)

const rootPlaceholder = "Sharpen Live alerts service (API disabled).\n"

// Options configures the HTTP router.
type Options struct {
	Logger             logging.Logger
	StreamersPath      string
	StreamersStore     *streamers.Store
	YouTube            config.YouTubeConfig
	AlertNotifications youtubehandlers.AlertNotificationOptions
}

// NewRouter constructs the HTTP router for the public API.
func NewRouter(opts Options) http.Handler {
	mux := http.NewServeMux()
	logger := opts.Logger
	streamersPath := opts.StreamersPath
	if streamersPath == "" {
		streamersPath = streamers.DefaultFilePath
	}
	streamersStore := opts.StreamersStore
	if streamersStore == nil {
		streamersStore = streamers.NewStore(streamersPath)
	}

	alertsOpts := opts.AlertNotifications
	if alertsOpts.Logger == nil {
		alertsOpts.Logger = logger
	}
	if alertsOpts.StreamersStore == nil {
		alertsOpts.StreamersStore = streamersStore
	}
	if alertsOpts.VideoLookup == nil {
		alertsOpts.VideoLookup = &liveinfo.Client{Logger: logger}
	}

	alertsHandler := handleAlerts(alertsOpts)
	mux.Handle("/alerts", alertsHandler)
	mux.Handle("/alert", alertsHandler)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, rootPlaceholder)
	})

	return logging.WithHTTPLogging(mux, logger)
}

// handleAlerts returns an HTTP handler that only treats likely Google/YouTube
// requests as WebSub subscription confirmations/notifications.
func handleAlerts(notificationOpts youtubehandlers.AlertNotificationOptions) http.Handler {
	allowedMethods := strings.Join([]string{http.MethodGet, http.MethodPost}, ", ")
	logger := notificationOpts.Logger
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alerts" && r.URL.Path != "/alert" {
			http.NotFound(w, r)
			return
		}

		userAgent := r.Header.Get("User-Agent")
		from := r.Header.Get("From")
		forwardedFor := r.Header.Get("X-Forwarded-For")
		platform := alertPlatform(userAgent, from)

		switch r.Method {
		case http.MethodGet:
			if platform == "youtube" {
				if youtubehandlers.HandleSubscriptionConfirmation(w, r, youtubehandlers.SubscriptionConfirmationOptions{
					Logger:         logger,
					StreamersStore: notificationOpts.StreamersStore,
				}) {
					return
				}
				http.Error(w, "invalid subscription confirmation", http.StatusBadRequest)
				return
			}
			if logger != nil {
				logger.Printf("suspicious /alerts GET request: platform=%q ua=%q from=%q xff=%q", platform, userAgent, from, forwardedFor)
			}
			w.Header().Set("Allow", allowedMethods)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		case http.MethodPost:
			if platform != "youtube" {
				if logger != nil {
					logger.Printf("suspicious /alerts POST request: platform=%q ua=%q from=%q xff=%q", platform, userAgent, from, forwardedFor)
				}
				w.Header().Set("Allow", allowedMethods)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if youtubehandlers.HandleAlertNotification(w, r, notificationOpts) {
				return
			}
			http.Error(w, "failed to process notification", http.StatusInternalServerError)
		default:
			w.Header().Set("Allow", allowedMethods)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func alertPlatform(userAgent, from string) string {
	if strings.HasPrefix(userAgent, "FeedFetcher-Google") && from == "googlebot(at)googlebot.com" {
		return "youtube"
	}
	return ""
}
