package main

import (
	"testing"
	"time"

	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

func TestRouterConstructionMatchesMainConfig(t *testing.T) {
	logger := logging.New()
	opts := apiv1.Options{
		Logger:        logger,
		StreamersPath: streamers.DefaultFilePath,
		RuntimeInfo: apiv1.RuntimeInfo{
			Name:        "live-stream-alerts",
			Addr:        "127.0.0.1",
			Port:        ":8880",
			ReadTimeout: (10 * time.Second).String(),
			DataPath:    streamers.DefaultFilePath,
		},
	}
	if router := apiv1.NewRouter(opts); router == nil {
		t.Fatalf("expected router to be constructed")
	}
}
