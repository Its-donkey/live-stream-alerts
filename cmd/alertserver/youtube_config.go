// file name — cmd/alertserver/youtube_config.go
package main

import (
	"flag"

	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

// configureYouTubeDefaults sets up YouTube WebSub flags, parses them, and
// applies the resulting values to the subscriptions defaults.
func configureYouTubeDefaults() {
	youtubeHubURL := flag.String(
		"youtube-hub-url",
		envOr("YOUTUBE_HUB_URL", subscriptions.DefaultHubURL),
		"YouTube PubSubHubbub hub URL.",
	)

	youtubeCallbackURL := flag.String(
		"youtube-callback-url",
		envOr("YOUTUBE_CALLBACK_URL", subscriptions.DefaultCallbackURL),
		"Callback URL registered with the hub.",
	)

	youtubeLeaseSeconds := flag.Int(
		"youtube-lease-seconds",
		envIntOr("YOUTUBE_LEASE_SECONDS", subscriptions.DefaultLease),
		"Lease duration (seconds) requested from the hub.",
	)

	youtubeMode := flag.String(
		"youtube-default-mode",
		envOr("YOUTUBE_DEFAULT_MODE", subscriptions.DefaultMode),
		"Default WebSub mode applied when not provided.",
	)

	youtubeVerify := flag.String(
		"youtube-verify-mode",
		envOr("YOUTUBE_VERIFY_MODE", subscriptions.DefaultVerify),
		"Default WebSub verify strategy.",
	)

	// Parse all CLI flags once.
	flag.Parse()

	// Apply the effective defaults into the subscriptions package.
	subscriptions.ConfigureDefaults(subscriptions.Defaults{
		HubURL:       *youtubeHubURL,
		CallbackURL:  *youtubeCallbackURL,
		LeaseSeconds: *youtubeLeaseSeconds,
		Mode:         *youtubeMode,
		Verify:       *youtubeVerify,
	})
}
