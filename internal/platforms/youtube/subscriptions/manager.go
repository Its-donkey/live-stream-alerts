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
	Client       *http.Client
	HubURL       string
	Logger       logging.Logger
	Mode         string // subscribe or unsubscribe; must be provided
	Verify       string
	LeaseSeconds int
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
	topic = strings.TrimSpace(yt.Topic)
	if topic == "" {
		topic = fmt.Sprintf("https://www.youtube.com/xml/feeds/videos.xml?channel_id=%s", channelID)
	}

	return channelID, topic, secret, nil
}

// ManageSubscription ensures the supplied streamer record is registered with the YouTube PubSubHubbub hub.
// When record.Platforms.YouTube is nil, this is a no-op.
func ManageSubscription(ctx context.Context, record streamers.Record, opts Options) error {
	if record.Platforms.YouTube == nil {
		return nil
	}
	yt := record.Platforms.YouTube

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

	verify := strings.TrimSpace(yt.VerifyMode)
	if verify == "" {
		verify = strings.TrimSpace(opts.Verify)
	}
	if verify == "" {
		verify = "async"
	}
	leaseSeconds := resolveLeaseSeconds(mode, yt, opts)
	hubURL := strings.TrimSpace(yt.HubURL)
	if hubURL == "" {
		hubURL = strings.TrimSpace(opts.HubURL)
	}
	callback := strings.TrimSpace(yt.CallbackURL)

	subscribeReq := YouTubeRequest{
		HubURL:       hubURL,
		Topic:        topic,
		Callback:     callback,
		Secret:       secret,
		Verify:       verify,
		ChannelID:    channelID,
		Mode:         mode,
		LeaseSeconds: leaseSeconds,
	}

	resp, body, finalReq, err := SubscribeYouTube(ctx, client, logger, subscribeReq)
	if err != nil {
		return fmt.Errorf("subscribe youtube alerts: %w", err)
	}

	if logger != nil && resp != nil {
		alias := strings.TrimSpace(record.Streamer.Alias)
		if alias == "" {
			alias = record.Streamer.ID
		}
		logger.Printf(
			"YouTube hub accepted %s for %s (topic=%s, callback=%s, status=%s). Awaiting hub challenge with token %s.",
			mode,
			alias,
			finalReq.Topic,
			finalReq.Callback,
			resp.Status,
			finalReq.VerifyToken,
		)
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

func resolveLeaseSeconds(mode string, yt *streamers.YouTubePlatform, opts Options) int {
	if !strings.EqualFold(mode, "subscribe") {
		return 0
	}
	if yt != nil && yt.LeaseSeconds > 0 {
		return yt.LeaseSeconds
	}
	if opts.LeaseSeconds > 0 {
		return opts.LeaseSeconds
	}
	return 0
}
