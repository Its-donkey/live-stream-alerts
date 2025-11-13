// file name — /cmd/alertserver/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/server"
)

func main() {
	logger := logging.New()
	const (
		addr = "127.0.0.1"
		port = ":8880"
	)
	var readWindow = 10 * time.Second

	serverHandler := apiv1.New(apiv1.Options{
		Logger:        logger,
		StreamersPath: "data/streamers.json",
		RuntimeInfo: apiv1.RuntimeInfo{
			Name:        "live-stream-alerts",
			Addr:        addr,
			Port:        port,
			ReadTimeout: readWindow.String(),
		},
	})

	s := server.Config{
		Addr:        addr,
		Port:        port,
		ReadTimeout: readWindow,
		Logger:      logger,
		Handler:     serverHandler,
	}
	srv, err := s.New()
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
		s.Logger.Printf("Shutting down...")
		_ = srv.Close()
	case err := <-errCh:
		if err != nil {
			s.Logger.Printf("Server error: %v", err)
			os.Exit(1)
		}
	}
}
