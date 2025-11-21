package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"live-stream-alerts/internal/platforms/youtube/liveinfo"
	"live-stream-alerts/internal/streamers"
)

// AlertProcessor orchestrates WebSub notification handling.
type AlertProcessor struct {
	Streamers   *streamers.Store
	VideoLookup LiveVideoLookup
}

// LiveVideoLookup fetches metadata for YouTube video IDs.
type LiveVideoLookup interface {
	Fetch(ctx context.Context, videoIDs []string) (map[string]liveinfo.VideoInfo, error)
}

// AlertProcessRequest describes an inbound WebSub feed.
type AlertProcessRequest struct {
	Feed       io.Reader
	RemoteAddr string
}

// AlertProcessResult captures the outcomes of processing a feed.
type AlertProcessResult struct {
	Entries       int
	VideoIDs      []string
	LiveUpdates   []LiveUpdate
	SkippedVideos []SkippedVideo
}

// LiveUpdate describes a streamer whose live status was updated.
type LiveUpdate struct {
	ChannelID string
	VideoID   string
	Title     string
	StartedAt time.Time
}

// SkippedVideo describes a video that could not be processed.
type SkippedVideo struct {
	VideoID string
	Reason  string
}

var (
	// ErrInvalidFeed indicates the Atom payload was malformed.
	ErrInvalidFeed = errors.New("invalid feed")
	// ErrLookupFailed indicates the video metadata lookup failed.
	ErrLookupFailed = errors.New("video lookup failed")
)

const maxFeedSize = 1 << 20 // 1MiB

// Process decodes the feed, fetches video metadata, and updates streamers.
func (p AlertProcessor) Process(ctx context.Context, req AlertProcessRequest) (AlertProcessResult, error) {
	if p.Streamers == nil {
		return AlertProcessResult{}, errors.New("streamers store is not configured")
	}
	if p.VideoLookup == nil {
		return AlertProcessResult{}, errors.New("video lookup is not configured")
	}
	if req.Feed == nil {
		return AlertProcessResult{}, fmt.Errorf("%w: feed reader is nil", ErrInvalidFeed)
	}

	var feed youtubeFeed
	decoder := xml.NewDecoder(io.LimitReader(req.Feed, maxFeedSize))
	if err := decoder.Decode(&feed); err != nil {
		return AlertProcessResult{}, fmt.Errorf("%w: %v", ErrInvalidFeed, err)
	}
	result := AlertProcessResult{Entries: len(feed.Entries)}
	if len(feed.Entries) == 0 {
		return result, nil
	}
	videoIDs := extractVideoIDs(feed)
	result.VideoIDs = videoIDs
	info, err := p.VideoLookup.Fetch(ctx, videoIDs)
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrLookupFailed, err)
	}
	for _, entry := range feed.Entries {
		id := strings.TrimSpace(entry.VideoID)
		channelID := strings.TrimSpace(entry.ChannelID)
		if id == "" || channelID == "" {
			continue
		}
		video, ok := info[id]
		if !ok {
			result.SkippedVideos = append(result.SkippedVideos, SkippedVideo{VideoID: id, Reason: "metadata missing"})
			continue
		}
		if !video.IsLive() {
			result.SkippedVideos = append(result.SkippedVideos, SkippedVideo{VideoID: id, Reason: "not live"})
			continue
		}
		startedAt := video.ActualStartTime
		if startedAt.IsZero() {
			startedAt = entry.Updated
		}
		_, updateErr := p.Streamers.UpdateYouTubeLiveStatus(channelID, streamers.YouTubeLiveStatus{
			Live:      true,
			VideoID:   id,
			StartedAt: startedAt,
		})
		if updateErr != nil {
			result.SkippedVideos = append(result.SkippedVideos, SkippedVideo{VideoID: id, Reason: updateErr.Error()})
			continue
		}
		result.LiveUpdates = append(result.LiveUpdates, LiveUpdate{
			ChannelID: channelID,
			VideoID:   id,
			Title:     entry.Title,
			StartedAt: startedAt,
		})
	}
	return result, nil
}

type youtubeFeed struct {
	Entries []youtubeEntry `xml:"entry"`
}

type youtubeEntry struct {
	VideoID   string    `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	ChannelID string    `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
	Title     string    `xml:"title"`
	Updated   time.Time `xml:"updated"`
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
