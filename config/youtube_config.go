// file name — /cmd/alertserver/youtube_config.go
package config

import (
	"flag"
	"os"
	"strconv"
	"strings"

	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

// activeConfig is the single place where you define YouTube WebSub configuration
// and its defaults. Flags and env vars override these values at startup.
var activeConfig = subscriptions.Defaults{
	HubURL:       "https://pubsubhubbub.appspot.com/subscribe",
	CallbackURL:  "https://sharpen.live/alerts",
	LeaseSeconds: 864000, // 10 days in seconds (maximum for YouTube)
	Mode:         "subscribe",
	Verify:       "async",
}

// ConfigureYouTube reads flags/env and pushes the final configuration into
// the subscriptions package. Call this from main() before starting the server.
func ConfigureYouTube() {
	youtubeHubURL := flag.String(
		"youtube-hub-url",
		envOr("YOUTUBE_HUB_URL", activeConfig.HubURL),
		"YouTube PubSubHubbub hub URL.",
	)

	youtubeCallbackURL := flag.String(
		"youtube-callback-url",
		envOr("YOUTUBE_CALLBACK_URL", activeConfig.CallbackURL),
		"Callback URL registered with the hub.",
	)

	youtubeLeaseSeconds := flag.Int(
		"youtube-lease-seconds",
		envIntOr("YOUTUBE_LEASE_SECONDS", activeConfig.LeaseSeconds),
		"Lease duration (seconds) requested from the hub.",
	)

	youtubeMode := flag.String(
		"youtube-default-mode",
		envOr("YOUTUBE_DEFAULT_MODE", activeConfig.Mode),
		"Default WebSub mode applied when not provided.",
	)

	youtubeVerify := flag.String(
		"youtube-verify-mode",
		envOr("YOUTUBE_VERIFY_MODE", activeConfig.Verify),
		"Default WebSub verify strategy.",
	)

	// Parse all CLI flags once (only do this in one place in your program).
	flag.Parse()

	// Update activeConfig with the final values.
	activeConfig = subscriptions.Defaults{
		HubURL:       *youtubeHubURL,
		CallbackURL:  *youtubeCallbackURL,
		LeaseSeconds: *youtubeLeaseSeconds,
		Mode:         *youtubeMode,
		Verify:       *youtubeVerify,
	}

	// Push the config into the subscriptions package.
	subscriptions.ConfigureDefaults(activeConfig)
}

func envOr(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
