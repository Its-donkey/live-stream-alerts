package main

import (
	"testing"

	"live-stream-alerts/config"
	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

func TestRouterConstructionMatchesMainConfig(t *testing.T) {
	logger := logging.New()
	opts := apiv1.Options{
		Logger:        logger,
		StreamersPath: streamers.DefaultFilePath,
		YouTube: config.YouTubeConfig{
			HubURL:       "https://hub.example.com",
			CallbackURL:  "https://callback.example.com/alerts",
			Verify:       "async",
			LeaseSeconds: 60,
		},
	}
	if router := apiv1.NewRouter(opts); router == nil {
		t.Fatalf("expected router to be constructed")
	}
}
