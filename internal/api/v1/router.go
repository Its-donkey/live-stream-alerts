package v1

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/config"
	adminauth "live-stream-alerts/internal/admin/auth"
	adminhttp "live-stream-alerts/internal/admin/http"
	"live-stream-alerts/internal/logging"
	youtubehandlers "live-stream-alerts/internal/platforms/youtube/handlers"
	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/streamers"
	streamershandlers "live-stream-alerts/internal/streamers/handlers"
	"live-stream-alerts/internal/submissions"
)

const rootPlaceholder = "UI assets not configured. Run the standalone alGUI project separately.\n"

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
	Logger             logging.Logger
	RuntimeInfo        RuntimeInfo
	StreamersPath      string
	SubmissionsPath    string
	AdminAuth          *adminauth.Manager
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
	submissionsPath := opts.SubmissionsPath
	if submissionsPath == "" {
		submissionsPath = submissions.DefaultFilePath
	}

	streamersHandler := streamershandlers.StreamersHandler(streamershandlers.StreamOptions{
		Logger:          logger,
		FilePath:        streamersPath,
		SubmissionsPath: submissionsPath,
		YouTubeHubURL:   strings.TrimSpace(opts.YouTube.HubURL),
	})

	mux.Handle("/api/youtube/channel", youtubehandlers.NewChannelLookupHandler(nil))
	mux.Handle("/api/youtube/metadata", youtubehandlers.NewMetadataHandler(youtubehandlers.MetadataHandlerOptions{}))
	commonSubOpts := youtubehandlers.SubscriptionHandlerOptions{
		Logger:       logger,
		HubURL:       strings.TrimSpace(opts.YouTube.HubURL),
		CallbackURL:  strings.TrimSpace(opts.YouTube.CallbackURL),
		VerifyMode:   strings.TrimSpace(opts.YouTube.Verify),
		LeaseSeconds: opts.YouTube.LeaseSeconds,
	}
	mux.Handle("/api/youtube/subscribe", youtubehandlers.NewSubscribeHandler(commonSubOpts))
	mux.Handle("/api/youtube/unsubscribe", youtubehandlers.NewUnsubscribeHandler(commonSubOpts))
	mux.Handle("/api/streamers", streamersHandler)
	mux.Handle("/api/streamers/watch", streamersWatchHandler(streamersWatchOptions{
		FilePath:     streamersPath,
		Logger:       logger,
		PollInterval: time.Second,
	}))

	alertsOpts := opts.AlertNotifications
	if alertsOpts.Logger == nil {
		alertsOpts.Logger = logger
	}
	if alertsOpts.StreamersPath == "" {
		alertsOpts.StreamersPath = streamersPath
	}
	if alertsOpts.VideoLookup == nil {
		alertsOpts.VideoLookup = &liveinfo.Client{Logger: logger}
	}

	alertsHandler := handleAlerts(alertsOpts)
	mux.Handle("/alerts", alertsHandler)
	mux.Handle("/alert", alertsHandler)

	if opts.AdminAuth != nil {
		mux.Handle("/api/admin/login", adminhttp.NewLoginHandler(opts.AdminAuth))
		mux.Handle("/api/admin/submissions", adminhttp.NewSubmissionsHandler(adminhttp.SubmissionsHandlerOptions{
			Manager:       opts.AdminAuth,
			FilePath:      submissionsPath,
			StreamersPath: streamersPath,
			Logger:        logger,
			YouTube:       opts.YouTube,
		}))
	}

	mux.HandleFunc("/api/server/config", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, opts.RuntimeInfo)
	})

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

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
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
					Logger:        logger,
					StreamersPath: notificationOpts.StreamersPath,
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
