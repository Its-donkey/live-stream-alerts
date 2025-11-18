package handlers

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/streamers"
)

// LiveVideoLookup fetches live metadata for video IDs.
type LiveVideoLookup interface {
	Fetch(ctx context.Context, videoIDs []string) (map[string]liveinfo.VideoInfo, error)
}

// AlertNotificationOptions configure POST /alerts handling.
type AlertNotificationOptions struct {
	Logger        logging.Logger
	StreamersPath string
	VideoLookup   LiveVideoLookup
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

	if opts.VideoLookup == nil {
		return false
	}

	var feed youtubeFeed
	decoder := xml.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&feed); err != nil {
		if opts.Logger != nil {
			opts.Logger.Printf("failed to decode hub notification from %s: %v", r.RemoteAddr, err)
		}
		http.Error(w, "invalid atom feed", http.StatusBadRequest)
		return true
	}

	if len(feed.Entries) == 0 {
		if opts.Logger != nil {
			opts.Logger.Printf("hub notification from %s contained no entries", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	videoIDs := extractVideoIDs(feed)
	if opts.Logger != nil {
		opts.Logger.Printf("Processing hub notification (%d entries) videos=%s channel=%s", len(feed.Entries), strings.Join(videoIDs, ","), feed.Entries[0].ChannelID)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	videoInfo, err := opts.VideoLookup.Fetch(ctx, videoIDs)
	if err != nil {
		if opts.Logger != nil {
			opts.Logger.Printf("failed to fetch live metadata for videos %s: %v", strings.Join(videoIDs, ","), err)
		}
		w.WriteHeader(http.StatusAccepted)
		return true
	}

	var liveUpdates int
	for _, entry := range feed.Entries {
		info, ok := videoInfo[entry.VideoID]
		if !ok || !info.IsLive() {
			if opts.Logger != nil {
				if !ok {
					opts.Logger.Printf("no metadata returned for video %s, skipping", entry.VideoID)
				} else {
					opts.Logger.Printf("video %s (%s) not live; broadcast=%q start=%s", entry.VideoID, entry.Title, info.LiveBroadcastContent, info.ActualStartTime)
				}
			}
			continue
		}
		startedAt := info.ActualStartTime
		if startedAt.IsZero() {
			startedAt = entry.Updated
		}
		_, err := streamers.UpdateYouTubeLiveStatus(opts.StreamersPath, entry.ChannelID, streamers.YouTubeLiveStatus{
			Live:      true,
			VideoID:   entry.VideoID,
			StartedAt: startedAt,
		})
		if err != nil && opts.Logger != nil {
			opts.Logger.Printf("failed to update live status for %s: %v", entry.ChannelID, err)
			continue
		}
		liveUpdates++
		if opts.Logger != nil {
			opts.Logger.Printf("Live stream detected: channel=%s video=%s title=%s", entry.ChannelID, entry.VideoID, entry.Title)
		}
	}

	if liveUpdates == 0 && opts.Logger != nil {
		opts.Logger.Printf("Processed alert notification for %d video(s); no live streams detected", len(feed.Entries))
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

type youtubeFeed struct {
	Entries []youtubeEntry `xml:"entry"`
}

type youtubeEntry struct {
	VideoID   string      `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID string      `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
	Title     string      `xml:"title"`
	Published time.Time   `xml:"published"`
	Updated   time.Time   `xml:"updated"`
	Link      entryLink   `xml:"link"`
	Author    entryAuthor `xml:"author"`
}

type entryLink struct {
	Href string `xml:"href,attr"`
}

type entryAuthor struct {
	Name string `xml:"name"`
	URI  string `xml:"uri"`
}

func extractVideoIDs(feed youtubeFeed) []string {
	ids := make([]string, 0, len(feed.Entries))
	seen := make(map[string]struct{}, len(feed.Entries))
	for _, entry := range feed.Entries {
		id := strings.TrimSpace(entry.VideoID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
