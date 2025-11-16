// file name — /subscriber.go
package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/websub"
	"live-stream-alerts/internal/streamers"
)

// Options configures how subscriptions are issued.
type Options struct {
	Client *http.Client
	HubURL string
	Logger logging.Logger
	Mode   string // subscribe or unsubscribe; must be provided
}

// getLogger returns an appropriate logger, defaulting when none is provided.
func (o Options) getLogger() logging.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return logging.New()
}

// buildYouTubeSubscriptionData centralises the shared logic for:
//   - reading YouTube details from the record
//   - resolving channel ID from handle if required
//   - validating that a channel ID exists
//   - constructing the topic URL
//   - extracting the hub secret
func buildYouTubeSubscriptionData(
	ctx context.Context,
	record streamers.Record,
	client *http.Client,
	action string, // e.g. "subscribe" or "unsubscribe" for clearer error messages
) (channelID, topic, secret string, err error) {
	if record.Platforms.YouTube == nil {
		return "", "", "", errors.New("record has no YouTube platform configured")
	}

	yt := record.Platforms.YouTube
	channelID = strings.TrimSpace(yt.ChannelID)
	handle := strings.TrimSpace(yt.Handle)

	if channelID == "" && handle != "" {
		resolvedID, resErr := ResolveChannelID(ctx, handle, client)
		if resErr != nil {
			return "", "", "", fmt.Errorf("resolve channel ID for handle %s: %w", handle, resErr)
		}
		channelID = resolvedID
	}

	if channelID == "" {
		if action == "" {
			action = "proceed"
		}
		return "", "", "", fmt.Errorf("youtube channel ID missing; cannot %s", action)
	}

	secret = strings.TrimSpace(yt.HubSecret)
	topic = fmt.Sprintf("https://www.youtube.com/xml/feeds/videos.xml?channel_id=%s", channelID)

	return channelID, topic, secret, nil
}

// ManageSubscription ensures the supplied streamer record is registered with the YouTube PubSubHubbub hub.
// When record.Platforms.YouTube is nil, this is a no-op.
func ManageSubscription(ctx context.Context, record streamers.Record, opts Options) error {
	if record.Platforms.YouTube == nil {
		return nil
	}

	mode := strings.TrimSpace(opts.Mode)
	if mode == "" {
		return errors.New("mode is required; set to subscribe or unsubscribe")
	}

	client := opts.Client // defaulting is handled in SubscribeYouTube
	logger := opts.getLogger()

	channelID, topic, secret, err := buildYouTubeSubscriptionData(ctx, record, client, mode)
	if err != nil {
		return err
	}

	subscribeReq := YouTubeRequest{
		HubURL:    strings.TrimSpace(opts.HubURL), // may be empty; SubscribeYouTube will fall back to config
		Topic:     topic,
		Secret:    secret,
		Verify:    "async",
		ChannelID: channelID,
		Mode:      mode,
		// Callback left empty here; SubscribeYouTube will use config.YT.CallbackURL
	}

	resp, body, finalReq, err := SubscribeYouTube(ctx, client, logger, subscribeReq)
	if err != nil {
		return fmt.Errorf("subscribe youtube alerts: %w", err)
	}

	websub.RecordSubscriptionResult(
		finalReq.VerifyToken,
		record.Streamer.Alias,
		topic,
		resp.Status,
		string(body),
	)
	return nil
}
