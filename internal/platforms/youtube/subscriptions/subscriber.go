package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
)

// Options configures how subscriptions are issued.
type Options struct {
	Client *http.Client
	HubURL string
	Logger logging.Logger
}

// Subscribe ensures the supplied streamer record is registered with the YouTube PubSubHubbub hub.
// When record.Platforms.YouTube is nil, this is a no-op.
func Subscribe(ctx context.Context, record streamers.Record, opts Options) error {
	if record.Platforms.YouTube == nil {
		return nil
	}

	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	hubURL := strings.TrimSpace(opts.HubURL)
	if hubURL == "" {
		hubURL = DefaultHubURL
	}

	yt := record.Platforms.YouTube
	channelID := strings.TrimSpace(yt.ChannelID)
	handle := strings.TrimSpace(yt.Handle)

	if channelID == "" && handle != "" {
		resolvedID, err := ResolveChannelID(ctx, handle, client)
		if err != nil {
			return fmt.Errorf("resolve channel ID for handle %s: %w", handle, err)
		}
		channelID = resolvedID
	}
	if channelID == "" {
		return errors.New("youtube channel ID missing; cannot subscribe")
	}

	secret := strings.TrimSpace(yt.HubSecret)

	topic := fmt.Sprintf("https://www.youtube.com/xml/feeds/videos.xml?channel_id=%s", channelID)
	subscribeReq := YouTubeRequest{
		Topic:     topic,
		Secret:    secret,
		Verify:    "async",
		ChannelID: channelID,
	}
	NormaliseSubscribeRequest(&subscribeReq)

	resp, body, finalReq, err := SubscribeYouTube(ctx, client, hubURL, subscribeReq)
	if err != nil {
		return fmt.Errorf("subscribe youtube alerts: %w", err)
	}

	websub.RecordSubscriptionResult(finalReq.VerifyToken, record.Streamer.Alias, topic, resp.Status, string(body))
	return nil
}
