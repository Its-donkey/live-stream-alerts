// file name — /cmd/alertserver/main.go
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	httpv1 "live-stream-alerts/internal/http/v1"
	"live-stream-alerts/internal/server"
)

func main() {
	_ = mime.AddExtensionType(".wasm", "application/wasm")

	logger := log.Default()
	const (
		addr = "127.0.0.1"
		port = ":8880"
	)
	var readWindow = 10 * time.Second

	serverHandler := httpv1.New(httpv1.Options{
		Logger:   logger,
		StaticFS: initStaticFS(logger),
		RuntimeInfo: httpv1.RuntimeInfo{
			Name:        "alGUI",
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

func initStaticFS(logger *log.Logger) fs.FS {
	const rel = "web/algui"

	if dir := os.Getenv("ALGUI_STATIC_DIR"); dir != "" {
		if fsys, err := dirFS(dir); err == nil {
			logger.Printf("alGUI assets loaded from %s", dir)
			return fsys
		}
		logger.Printf("failed to use $ALGUI_STATIC_DIR %q", dir)
	}

	candidates := []string{rel}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", rel),
			filepath.Join(exeDir, "..", "..", rel),
		)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if fsys, err := dirFS(candidate); err == nil {
			if logger != nil {
				logger.Printf("alGUI assets loaded from %s", candidate)
			}
			return fsys
		}
	}

	if logger != nil {
		logger.Printf("alGUI assets directory not found; UI fallback active")
	}
	return nil
}

func dirFS(path string) (fs.FS, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return os.DirFS(abs), nil
}
