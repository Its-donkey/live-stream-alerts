package handlers

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/api"
)

const defaultNotificationLimit = 1 << 20 // 1 MiB

// NotificationOptions configures how WebSub notifications are processed.
type NotificationOptions struct {
	Logger       logging.Logger
	HTTPClient   *http.Client
	StatusClient LiveStatusClient
	BodyLimit    int64
}

// LiveStatusClient describes the subset of the player API used by notifications.
type LiveStatusClient interface {
	LiveStatus(ctx context.Context, videoID string) (api.LiveStatus, error)
}

// HandleNotification ingests POSTed PubSubHubbub notifications from YouTube.
func HandleNotification(w http.ResponseWriter, r *http.Request, opts NotificationOptions) bool {
	if r.Method != http.MethodPost {
		return false
	}
	defer r.Body.Close()

	limit := opts.BodyLimit
	if limit <= 0 {
		limit = defaultNotificationLimit
	}
	lr := &io.LimitedReader{R: r.Body, N: limit}
	body, err := io.ReadAll(lr)
	if err != nil {
		http.Error(w, "failed to read notification", http.StatusInternalServerError)
		return true
	}
	if lr.N == 0 {
		http.Error(w, "notification payload too large", http.StatusRequestEntityTooLarge)
		return true
	}

	feed, err := parseNotificationFeed(body)
	if err != nil {
		http.Error(w, "invalid Atom feed", http.StatusBadRequest)
		return true
	}
	if len(feed.Entries) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	client := opts.StatusClient
	if client == nil {
		client = api.NewPlayerClient(api.PlayerClientOptions{HTTPClient: opts.HTTPClient})
	}

	for _, entry := range feed.Entries {
		if entry.VideoID == "" {
			logNotification(opts.Logger, entry, api.LiveStatus{}, fmt.Errorf("missing video id"))
			continue
		}
		status, err := client.LiveStatus(r.Context(), entry.VideoID)
		logNotification(opts.Logger, entry, status, err)
	}

	w.WriteHeader(http.StatusNoContent)
	return true
}

func parseNotificationFeed(data []byte) (notificationFeed, error) {
	var feed notificationFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return feed, err
	}
	return feed, nil
}

func logNotification(logger logging.Logger, entry notificationEntry, status api.LiveStatus, err error) {
	if logger == nil {
		return
	}
	title := entry.Title
	if title == "" {
		title = status.Title
	}
	if title == "" {
		title = entry.VideoID
	}

	if err != nil {
		logger.Printf("YouTube notification for %s failed: %v", title, err)
		return
	}

	channelID := entry.ChannelID
	if channelID == "" {
		channelID = status.ChannelID
	}

	if status.IsOnline() {
		logger.Printf("YouTube livestream online: %s (video=%s channel=%s)", title, status.VideoID, channelID)
		return
	}
	if status.IsLive {
		logger.Printf("YouTube livestream offline: %s (video=%s liveNow=%t status=%s)", title, status.VideoID, status.IsLiveNow, status.PlayabilityStatus)
		return
	}
	logger.Printf("YouTube upload ignored (not live): %s (video=%s)", title, status.VideoID)
}

type notificationFeed struct {
	Entries []notificationEntry `xml:"entry"`
}

type notificationEntry struct {
	Title     string `xml:"title"`
	VideoID   string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID string `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
}
