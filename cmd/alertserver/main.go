package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"live-stream-alerts/config"
	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/httpserver"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

func main() {
	logFile, err := configureLogging()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logging: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	config.MustLoad("config.json")
	logger := logging.New()
	const (
		addr = "127.0.0.1"
		port = ":8880"
	)
	readWindow := 10 * time.Second

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

func configureLogging() (*os.File, error) {
	const logFileName = "alertserver.log"
	logPath := filepath.Join("data", logFileName)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	logging.SetDefaultWriter(io.MultiWriter(os.Stdout, file))
	return file, nil
}
