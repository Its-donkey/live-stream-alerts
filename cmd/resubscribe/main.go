package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	youtubesubscriber "live-stream-alerts/internal/platforms/youtube/subscriber"
	"live-stream-alerts/internal/streamers"
)

func main() {
	var (
		streamersPath = flag.String("path", "data/streamers.json", "path to streamers.json")
		aliasFilter   = flag.String("alias", "", "only resubscribe records matching this alias (optional)")
		hubURL        = flag.String("hub-url", "", "override the default YouTube hub URL (optional)")
		timeout       = flag.Duration("timeout", 10*time.Second, "timeout per subscription request")
	)
	flag.Parse()

	logger := logging.New()

	records, err := streamers.List(*streamersPath)
	if err != nil {
		logger.Printf("failed to load streamers: %v", err)
		os.Exit(1)
	}

	targetAlias := strings.TrimSpace(*aliasFilter)
	if targetAlias != "" {
		targetAlias = strings.ToLower(targetAlias)
	}

	opts := youtubesubscriber.Options{
		HubURL: *hubURL,
		Logger: logger,
	}

	var resubCount int

	for _, record := range records {
		if record.Platforms.YouTube == nil {
			continue
		}
		if targetAlias != "" && strings.ToLower(record.Streamer.Alias) != targetAlias && strings.ToLower(record.Streamer.ID) != targetAlias {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		err := youtubesubscriber.Subscribe(ctx, record, opts)
		cancel()
		if err != nil {
			logger.Printf("failed to resubscribe %s: %v", record.Streamer.Alias, err)
			continue
		}
		resubCount++
	}

	if targetAlias != "" && resubCount == 0 {
		logger.Printf("no streamer matched alias %q", targetAlias)
	}
	logger.Printf("Resubscribe complete. Total updated: %d", resubCount)
}
