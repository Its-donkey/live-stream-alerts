package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"live-stream-alerts/config"
	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/httpserver"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/streamers"
)

const (
	defaultConfigPath  = "config.json"
	defaultLogDir      = "data"
	defaultLogFileName = "alertserver.log"
	defaultReadTimeout = 10 * time.Second
)

// Options controls how the application boots and where it loads configuration from.
type Options struct {
	ConfigPath  string
	LogDir      string
	LogFile     string
	ReadTimeout time.Duration
}

// Run wires dependencies together and blocks until the provided context is cancelled
// or the HTTP server exits with an error.
func Run(ctx context.Context, opts Options) error {
	if ctx == nil {
		return errors.New("context is required")
	}

	opts = opts.withDefaults()

	logFilePath := filepath.Join(opts.LogDir, opts.LogFile)
	logFile, err := configureLogging(logFilePath)
	if err != nil {
		return fmt.Errorf("configure logging: %w", err)
	}
	defer logFile.Close()

	appCfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	logger := logging.New()

	streamerStore := streamers.NewStore(streamers.DefaultFilePath)

	router := apiv1.NewRouter(apiv1.Options{
		Logger:         logger,
		StreamersPath:  streamerStore.Path(),
		StreamersStore: streamerStore,
		YouTube:        appCfg.YouTube,
	})

	serverCfg := httpserver.Config{
		Addr:        appCfg.Server.Addr,
		Port:        appCfg.Server.Port,
		ReadTimeout: opts.ReadTimeout,
		Logger:      logger,
		Handler:     router,
	}
	srv, err := httpserver.New(serverCfg)
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	monitorOpts := subscriptions.Options{
		Client:       &http.Client{Timeout: 10 * time.Second},
		HubURL:       appCfg.YouTube.HubURL,
		Logger:       logger,
		Mode:         "subscribe",
		Verify:       appCfg.YouTube.Verify,
		LeaseSeconds: appCfg.YouTube.LeaseSeconds,
	}
	monitor := subscriptions.StartLeaseMonitor(ctx, subscriptions.LeaseMonitorConfig{
		StreamersPath: streamerStore.Path(),
		Interval:      time.Minute,
		Options:       monitorOpts,
	})
	defer monitor.Stop()

	select {
	case <-ctx.Done():
		logger.Printf("Shutting down...")
		_ = srv.Close()
		if err := <-errCh; err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	}
}

func (o Options) withDefaults() Options {
	if o.ConfigPath == "" {
		o.ConfigPath = defaultConfigPath
	}
	if o.LogDir == "" {
		o.LogDir = defaultLogDir
	}
	if o.LogFile == "" {
		o.LogFile = defaultLogFileName
	}
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = defaultReadTimeout
	}
	return o
}
