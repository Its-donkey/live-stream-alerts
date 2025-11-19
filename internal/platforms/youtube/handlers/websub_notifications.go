package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	youtubeservice "live-stream-alerts/internal/platforms/youtube/service"
	"live-stream-alerts/internal/streamers"
)

type alertProcessor interface {
	Process(ctx context.Context, req youtubeservice.AlertProcessRequest) (youtubeservice.AlertProcessResult, error)
}

// AlertNotificationOptions configure POST /alerts handling.
type AlertNotificationOptions struct {
	Logger         logging.Logger
	StreamersStore *streamers.Store
	VideoLookup    youtubeservice.LiveVideoLookup
	Processor      alertProcessor
}

// HandleAlertNotification processes YouTube hub POST notifications.
func HandleAlertNotification(w http.ResponseWriter, r *http.Request, opts AlertNotificationOptions) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/alert", "/alerts":
	default:
		return false
	}

	proc := opts.Processor
	if proc == nil {
		if opts.VideoLookup == nil || opts.StreamersStore == nil {
			return false
		}
		proc = &youtubeservice.AlertProcessor{
			Streamers:   opts.StreamersStore,
			VideoLookup: opts.VideoLookup,
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := proc.Process(ctx, youtubeservice.AlertProcessRequest{
		Feed:       io.LimitReader(r.Body, 1<<20),
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
		handleAlertError(w, err, result, opts.Logger)
		return true
	}

	if opts.Logger != nil {
		if len(result.LiveUpdates) == 0 {
			opts.Logger.Printf("Processed alert notification for %d video(s); no live streams detected", result.Entries)
		} else {
			opts.Logger.Printf("Processed alert notification (%d entries); live streams=%d videos=%s",
				result.Entries,
				len(result.LiveUpdates),
				strings.Join(result.VideoIDs, ","),
			)
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

func handleAlertError(w http.ResponseWriter, err error, result youtubeservice.AlertProcessResult, logger logging.Logger) {
	switch {
	case errors.Is(err, youtubeservice.ErrInvalidFeed):
		http.Error(w, "invalid atom feed", http.StatusBadRequest)
	case errors.Is(err, youtubeservice.ErrLookupFailed):
		if logger != nil && len(result.VideoIDs) > 0 {
			logger.Printf("failed to fetch live metadata for videos %s: %v", strings.Join(result.VideoIDs, ","), err)
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		if logger != nil {
			logger.Printf("failed to process notification: %v", err)
		}
		http.Error(w, "failed to process notification", http.StatusInternalServerError)
	}
}
