package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/streamers"
)

// Options configures how YouTube onboarding should behave.
type Options struct {
	Client       *http.Client
	HubURL       string
	CallbackURL  string
	VerifyMode   string
	LeaseSeconds int
	Logger       logging.Logger
	Store        *streamers.Store
}

// FromURL parses the provided channel URL, resolves missing metadata, updates the streamer record,
// and triggers a WebSub subscription.
func FromURL(ctx context.Context, record streamers.Record, channelURL string, opts Options) error {
	channelURL = strings.TrimSpace(channelURL)
	if channelURL == "" {
		return errors.New("channel URL is required")
	}
	store := opts.Store
	if store == nil {
		return errors.New("streamers store is required")
	}

	handle, channelID, err := parseYouTubeURL(channelURL)
	if err != nil {
		return err
	}

	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	if channelID == "" && handle != "" {
		resolved, err := subscriptions.ResolveChannelID(ctx, handle, client)
		if err != nil {
			return fmt.Errorf("resolve channel ID from handle %s: %w", handle, err)
		}
		channelID = resolved
	}
	if channelID == "" {
		return errors.New("could not determine YouTube channel ID from URL")
	}

	hubSecret := generateHubSecret()

	topic := fmt.Sprintf("https://www.youtube.com/xml/feeds/videos.xml?channel_id=%s", channelID)
	callbackURL := strings.TrimSpace(opts.CallbackURL)
	if callbackURL == "" {
		return errors.New("callback URL is required")
	}
	hubURL := strings.TrimSpace(opts.HubURL)
	if hubURL == "" {
		return errors.New("hub URL is required")
	}
	verifyMode := strings.TrimSpace(opts.VerifyMode)
	if verifyMode == "" {
		verifyMode = "async"
	}
	leaseSeconds := opts.LeaseSeconds
	if leaseSeconds <= 0 {
		return errors.New("lease seconds must be positive")
	}

	updatedRecord, err := setYouTubePlatform(store, record.Streamer.ID, streamers.YouTubePlatform{
		Handle:       handle,
		ChannelID:    channelID,
		HubSecret:    hubSecret,
		Topic:        topic,
		CallbackURL:  callbackURL,
		HubURL:       hubURL,
		VerifyMode:   verifyMode,
		LeaseSeconds: leaseSeconds,
	})
	if err != nil {
		return err
	}

	subscribeOpts := subscriptions.Options{
		Client:       client,
		HubURL:       hubURL,
		Logger:       opts.Logger,
		Mode:         "subscribe",
		LeaseSeconds: leaseSeconds,
		Verify:       verifyMode,
	}
	return subscriptions.ManageSubscription(ctx, updatedRecord, subscribeOpts)
}

func parseYouTubeURL(raw string) (handle string, channelID string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid youtube url: %w", err)
	}
	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if strings.HasPrefix(segment, "@") {
			handle = segment
			break
		}
	}
	for i := 0; i < len(segments)-1; i++ {
		if strings.EqualFold(segments[i], "channel") {
			channelID = segments[i+1]
			break
		}
	}
	if channelID == "" {
		q := u.Query().Get("channel_id")
		if q != "" {
			channelID = q
		}
	}
	handle = strings.TrimSpace(handle)
	channelID = strings.TrimSpace(channelID)
	return handle, channelID, nil
}

func setYouTubePlatform(store *streamers.Store, streamerID string, yt streamers.YouTubePlatform) (streamers.Record, error) {
	var updated streamers.Record
	err := store.UpdateFile(func(file *streamers.File) error {
		for i := range file.Records {
			if !strings.EqualFold(file.Records[i].Streamer.ID, streamerID) {
				continue
			}
			copy := yt
			file.Records[i].Platforms.YouTube = &copy
			file.Records[i].UpdatedAt = time.Now().UTC()
			updated = file.Records[i]
			return nil
		}
		return fmt.Errorf("streamer %s not found", streamerID)
	})
	if err != nil {
		return streamers.Record{}, err
	}
	return updated, nil
}

func generateHubSecret() string {
	const secretBytes = 24
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
