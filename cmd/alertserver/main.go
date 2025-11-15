package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/httpserver"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

func main() {
	logger := logging.New()
	const (
		addr = "127.0.0.1"
		port = ":8880"
	)
	readWindow := 10 * time.Second

	// Configure YouTube WebSub defaults (flags + env).
	configureYouTubeDefaults()

	// -----------------------------------------------------
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
