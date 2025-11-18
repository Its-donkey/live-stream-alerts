package subscriptions

import (
	"context"
	"strings"
	"sync"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

// LeaseMonitorConfig configures the background YouTube lease renewal watcher.
type LeaseMonitorConfig struct {
	StreamersPath string
	Interval      time.Duration
	RenewWindow   float64
	Options       Options
	Now           func() time.Time
	Renew         func(context.Context, streamers.Record, Options) error
}

const defaultRenewWindow = 0.05

// LeaseMonitor periodically inspects stored YouTube subscriptions and renews them
// before their lease expires.
type LeaseMonitor struct {
	cfg          LeaseMonitorConfig
	options      Options
	logger       logging.Logger
	lastAttempts map[string]time.Time
	mu           sync.Mutex
}

// StartLeaseMonitor launches the lease monitor using the provided context.
func StartLeaseMonitor(ctx context.Context, cfg LeaseMonitorConfig) *LeaseMonitor {
	monitor := newLeaseMonitor(cfg)
	go monitor.run(ctx)
	return monitor
}

func newLeaseMonitor(cfg LeaseMonitorConfig) *LeaseMonitor {
	if cfg.StreamersPath == "" {
		cfg.StreamersPath = streamers.DefaultFilePath
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.RenewWindow <= 0 || cfg.RenewWindow >= 1 {
		cfg.RenewWindow = defaultRenewWindow
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Renew == nil {
		cfg.Renew = ManageSubscription
	}

	opts := cfg.Options
	if !strings.EqualFold(opts.Mode, "subscribe") {
		opts.Mode = "subscribe"
	}
	logger := opts.getLogger()
	if logger == nil {
		logger = logging.New()
	}

	return &LeaseMonitor{
		cfg:          cfg,
		options:      opts,
		logger:       logger,
		lastAttempts: make(map[string]time.Time),
	}
}

func (m *LeaseMonitor) run(ctx context.Context) {
	m.evaluate(ctx)

	ticker := time.NewTicker(m.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.evaluate(ctx)
		}
	}
}

func (m *LeaseMonitor) evaluate(ctx context.Context) {
	records, err := streamers.List(m.cfg.StreamersPath)
	if err != nil {
		if m.logger != nil {
			m.logger.Printf("lease monitor: failed to read streamers file: %v", err)
		}
		return
	}

	now := m.cfg.Now().UTC()
	for _, record := range records {
		m.inspectRecord(ctx, record, now)
	}
}

func (m *LeaseMonitor) inspectRecord(ctx context.Context, record streamers.Record, now time.Time) {
	yt := record.Platforms.YouTube
	if yt == nil {
		return
	}
	channelID := strings.TrimSpace(yt.ChannelID)
	if channelID == "" {
		return
	}

	leaseSeconds := resolveLeaseSeconds("subscribe", yt, m.options)
	if leaseSeconds <= 0 {
		return
	}
	leaseStart := strings.TrimSpace(yt.HubLeaseDate)
	if leaseStart == "" {
		return
	}
	startTime, err := time.Parse(time.RFC3339, leaseStart)
	if err != nil {
		if m.logger != nil {
			m.logger.Printf("lease monitor: invalid hubLeaseDate for %s: %v", channelID, err)
		}
		return
	}

	if !m.shouldRenew(startTime, leaseSeconds, now) {
		m.clearAttemptIfLeaseAdvanced(channelID, startTime)
		return
	}

	if m.awaitingRenewal(channelID, startTime) {
		return
	}
	m.recordAttempt(channelID, now)
	go m.triggerRenewal(ctx, record)
}

func (m *LeaseMonitor) shouldRenew(leaseStart time.Time, leaseSeconds int, now time.Time) bool {
	if leaseSeconds <= 0 {
		return false
	}
	leaseDuration := time.Duration(leaseSeconds) * time.Second
	margin := time.Duration(float64(leaseDuration) * m.cfg.RenewWindow)
	if margin <= 0 || margin >= leaseDuration {
		margin = leaseDuration / 20
		if margin <= 0 {
			margin = time.Second
		}
	}
	renewAt := leaseStart.Add(leaseDuration - margin)
	return !now.Before(renewAt)
}

func (m *LeaseMonitor) awaitingRenewal(channelID string, leaseStart time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	last, ok := m.lastAttempts[channelID]
	if !ok {
		return false
	}
	if leaseStart.After(last) {
		delete(m.lastAttempts, channelID)
		return false
	}
	return true
}

func (m *LeaseMonitor) clearAttemptIfLeaseAdvanced(channelID string, leaseStart time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.lastAttempts[channelID]; ok && leaseStart.After(last) {
		delete(m.lastAttempts, channelID)
	}
}

func (m *LeaseMonitor) recordAttempt(channelID string, at time.Time) {
	m.mu.Lock()
	m.lastAttempts[channelID] = at
	m.mu.Unlock()
}

func (m *LeaseMonitor) triggerRenewal(ctx context.Context, record streamers.Record) {
	if m.logger != nil {
		m.logger.Printf("lease monitor: renewing subscription for %s (channel=%s)", record.Streamer.Alias, record.Platforms.YouTube.ChannelID)
	}
	renewCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := m.cfg.Renew(renewCtx, record, m.options); err != nil && m.logger != nil {
		m.logger.Printf("lease monitor: renewal failed for %s: %v", record.Streamer.Alias, err)
	}
}
