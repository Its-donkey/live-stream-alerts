package subscriptions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"live-stream-alerts/internal/streamers"
)

func TestLeaseMonitorRenewsWhenWindowReached(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "streamers.json")
	leaseStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeStreamersFile(t, path, leaseStart, 100)

	var mu sync.Mutex
	renewed := 0
	done := make(chan struct{}, 1)
	monitor := newLeaseMonitor(LeaseMonitorConfig{
		StreamersPath: path,
		Interval:      time.Hour,
		RenewWindow:   0.05,
		Options:       Options{},
		Now: func() time.Time {
			return leaseStart.Add(95 * time.Second)
		},
		Renew: func(ctx context.Context, record streamers.Record, opts Options) error {
			mu.Lock()
			renewed++
			mu.Unlock()
			done <- struct{}{}
			return nil
		},
	})

	monitor.evaluate(context.Background())
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected renewal to trigger")
	}
	mu.Lock()
	defer mu.Unlock()
	if renewed != 1 {
		t.Fatalf("expected renewal count 1, got %d", renewed)
	}
}

func TestLeaseMonitorSkipsUntilWindow(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "streamers.json")
	leaseStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeStreamersFile(t, path, leaseStart, 200)

	monitor := newLeaseMonitor(LeaseMonitorConfig{
		StreamersPath: path,
		RenewWindow:   0.05,
		Options:       Options{},
		Now: func() time.Time {
			return leaseStart.Add(100 * time.Second)
		},
		Renew: func(ctx context.Context, record streamers.Record, opts Options) error {
			t.Fatalf("renew should not trigger before window")
			return nil
		},
	})

	monitor.evaluate(context.Background())
}

func TestLeaseMonitorOnlyRenewsOncePerLease(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "streamers.json")
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeStreamersFile(t, path, start, 100)

	current := start.Add(96 * time.Second)
	var mu sync.Mutex
	renewed := 0
	events := make(chan struct{}, 4)
	monitor := newLeaseMonitor(LeaseMonitorConfig{
		StreamersPath: path,
		RenewWindow:   0.05,
		Options:       Options{},
		Now: func() time.Time {
			return current
		},
		Renew: func(ctx context.Context, record streamers.Record, opts Options) error {
			mu.Lock()
			renewed++
			mu.Unlock()
			events <- struct{}{}
			return nil
		},
	})

	ctx := context.Background()
	monitor.evaluate(ctx)
	waitEvent(t, events)
	monitor.evaluate(ctx)
	assertNoEvent(t, events)

	// Simulate hub confirmation updating lease date.
	newStart := start.Add(200 * time.Second)
	writeStreamersFile(t, path, newStart, 100)
	current = newStart.Add(96 * time.Second)
	monitor.evaluate(ctx)
	waitEvent(t, events)
	mu.Lock()
	defer mu.Unlock()
	if renewed != 2 {
		t.Fatalf("expected renewal after lease advanced, got %d attempts", renewed)
	}
}

func TestLeaseMonitorStopWaitsForRenewals(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "streamers.json")
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeStreamersFile(t, path, start, 100)

	renewStarted := make(chan struct{})
	renewRelease := make(chan struct{})
	cfg := LeaseMonitorConfig{
		StreamersPath: path,
		Interval:      10 * time.Millisecond,
		RenewWindow:   0.05,
		Options:       Options{},
		Now: func() time.Time {
			return start.Add(95 * time.Second)
		},
		Renew: func(ctx context.Context, record streamers.Record, opts Options) error {
			select {
			case <-renewStarted:
			default:
				close(renewStarted)
			}
			<-renewRelease
			return ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	monitor := StartLeaseMonitor(ctx, cfg)
	t.Cleanup(func() { monitor.Stop() })

	select {
	case <-renewStarted:
	case <-time.After(time.Second):
		t.Fatalf("expected renewal to start")
	}
	cancel()
	close(renewRelease)
	monitor.Stop() // should wait for renewal goroutine to exit without panic.
}

func waitEvent(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("expected renewal event")
	}
}

func assertNoEvent(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected renewal event")
	default:
	}
}

func writeStreamersFile(t *testing.T, path string, leaseStart time.Time, leaseSeconds int) {
	t.Helper()
	file := streamers.File{
		SchemaRef: streamers.DefaultSchemaPath,
		Records: []streamers.Record{
			{
				Streamer: streamers.Streamer{
					ID:    "demo",
					Alias: "Demo",
				},
				Platforms: streamers.Platforms{
					YouTube: &streamers.YouTubePlatform{
						ChannelID:    "UC123",
						HubLeaseDate: leaseStart.Format(time.RFC3339),
						LeaseSeconds: leaseSeconds,
						CallbackURL:  "https://example.com/callback",
					},
				},
			},
		},
	}
	data, err := jsonMarshal(file)
	if err != nil {
		t.Fatalf("marshal streamers file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write streamers file: %v", err)
	}
}

func jsonMarshal(file streamers.File) ([]byte, error) {
	type alias streamers.File
	return json.MarshalIndent(alias(file), "", "  ")
}
