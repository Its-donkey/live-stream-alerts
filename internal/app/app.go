package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"live-stream-alerts/config"
	adminauth "live-stream-alerts/internal/admin/auth"
	apiv1 "live-stream-alerts/internal/api/v1"
	"live-stream-alerts/internal/httpserver"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/streamers"
	streamersvc "live-stream-alerts/internal/streamers/service"
	"live-stream-alerts/internal/submissions"
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
	submissionStore := submissions.NewStore(submissions.DefaultFilePath)
	streamerService := streamersvc.New(streamersvc.Options{
		Streamers:     streamerStore,
		Submissions:   submissionStore,
		YouTubeClient: &http.Client{Timeout: 10 * time.Second},
		YouTubeHubURL: strings.TrimSpace(appCfg.YouTube.HubURL),
	})

	adminAuth := buildAdminManager(appCfg.Admin)

	router := apiv1.NewRouter(apiv1.Options{
		Logger:           logger,
		StreamersPath:    streamerStore.Path(),
		StreamersStore:   streamerStore,
		SubmissionsPath:  submissionStore.Path(),
		SubmissionsStore: submissionStore,
		StreamersService: streamerService,
		AdminAuth:        adminAuth,
		YouTube:          appCfg.YouTube,
		RuntimeInfo: apiv1.RuntimeInfo{
			Name:        "live-stream-alerts",
			Addr:        appCfg.Server.Addr,
			Port:        appCfg.Server.Port,
			ReadTimeout: opts.ReadTimeout.String(),
			DataPath:    streamers.DefaultFilePath,
		},
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

	monitorCtx, cancelMonitor := context.WithCancel(ctx)
	defer cancelMonitor()

	monitorOpts := subscriptions.Options{
		Client:       &http.Client{Timeout: 10 * time.Second},
		HubURL:       appCfg.YouTube.HubURL,
		Logger:       logger,
		Mode:         "subscribe",
		Verify:       appCfg.YouTube.Verify,
		LeaseSeconds: appCfg.YouTube.LeaseSeconds,
	}
	subscriptions.StartLeaseMonitor(monitorCtx, subscriptions.LeaseMonitorConfig{
		StreamersPath: streamerStore.Path(),
		Interval:      time.Minute,
		Options:       monitorOpts,
	})

	select {
	case <-ctx.Done():
		logger.Printf("Shutting down...")
		cancelMonitor()
		_ = srv.Close()
		if err := <-errCh; err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		cancelMonitor()
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

func buildAdminManager(cfg config.AdminConfig) *adminauth.Manager {
	email := strings.TrimSpace(cfg.Email)
	if email == "" || strings.TrimSpace(cfg.Password) == "" {
		return nil
	}
	return adminauth.NewManager(adminauth.Config{
		Email:    email,
		Password: cfg.Password,
		TokenTTL: time.Duration(cfg.TokenTTLSeconds) * time.Second,
	})
}
