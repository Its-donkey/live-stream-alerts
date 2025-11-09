// file name — /cmd/alertserver/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpv1 "live-stream-alerts/internal/http/V1"
	"live-stream-alerts/internal/server"
)

func main() {
	logger := log.Default()
	s := server.Config{
		Addr:        "127.0.0.1",
		Port:        ":8880",
		ReadTimeout: 10 * time.Second,
		Logger:      logger,
		Handler:     httpv1.New(logger),
	}
	srv, err := s.New()
	if err != nil {
		log.Fatalf("Failed to build server: %v", err)
	}

	// Run server in background
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	// Graceful shutdown on Ctrl+C
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		s.Logger.Println("Shutting down...")
		_ = srv.Close()
	case err := <-errCh:
		if err != nil {
			s.Logger.Printf("Server error: %v", err)
			os.Exit(1)
		}
	}
}
