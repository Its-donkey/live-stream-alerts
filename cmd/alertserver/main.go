package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/httpserver"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/streamers"
)

func main() {
	logger := logging.New()
	const (
		addr = "127.0.0.1"
		port = ":8880"
	)
	readWindow := 10 * time.Second

	youtubeHubURL := flag.String("youtube-hub-url", envOr("YOUTUBE_HUB_URL", subscriptions.DefaultHubURL), "YouTube PubSubHubbub hub URL.")
	youtubeCallbackURL := flag.String("youtube-callback-url", envOr("YOUTUBE_CALLBACK_URL", subscriptions.DefaultCallbackURL), "Callback URL registered with the hub.")
	youtubeLeaseSeconds := flag.Int("youtube-lease-seconds", envIntOr("YOUTUBE_LEASE_SECONDS", subscriptions.DefaultLease), "Lease duration (seconds) requested from the hub.")
	youtubeMode := flag.String("youtube-default-mode", envOr("YOUTUBE_DEFAULT_MODE", subscriptions.DefaultMode), "Default WebSub mode applied when not provided.")
	youtubeVerify := flag.String("youtube-verify-mode", envOr("YOUTUBE_VERIFY_MODE", subscriptions.DefaultVerify), "Default WebSub verify strategy.")

	flag.Parse()

	subscriptions.ConfigureDefaults(subscriptions.Defaults{
		HubURL:       *youtubeHubURL,
		CallbackURL:  *youtubeCallbackURL,
		LeaseSeconds: *youtubeLeaseSeconds,
		Mode:         *youtubeMode,
		Verify:       *youtubeVerify,
	})

	router := apiv1.NewRouter(apiv1.Options{
		Logger:        logger,
		StreamersPath: streamers.DefaultFilePath,
		RuntimeInfo: apiv1.RuntimeInfo{
			Name:        "live-stream-alerts",
			Addr:        addr,
			Port:        port,
			ReadTimeout: readWindow.String(),
			DataPath:    streamers.DefaultFilePath,
		},
	})

	cfg := httpserver.Config{
		Addr:        addr,
		Port:        port,
		ReadTimeout: readWindow,
		Logger:      logger,
		Handler:     router,
	}
	srv, err := httpserver.New(cfg)
	if err != nil {
		logger.Printf("Failed to build server: %v", err)
		os.Exit(1)
	}

	// Run server in background
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	// Graceful shutdown on Ctrl+C
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		cfg.Logger.Printf("Shutting down...")
		_ = srv.Close()
	case err := <-errCh:
		if err != nil {
			cfg.Logger.Printf("Server error: %v", err)
			os.Exit(1)
		}
	}
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
