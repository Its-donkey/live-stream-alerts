package service_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	adminservice "live-stream-alerts/internal/admin/service"
	"live-stream-alerts/internal/streamers"
)

func TestMonitorServiceOverview(t *testing.T) {
	dir := t.TempDir()
	store := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	appendRecord(t, store, "healthy", "UChealthy12345678901234", now.Add(-2*time.Minute), 600)
	appendRecord(t, store, "renewing", "UCrenewing123456789012", now.Add(-9*time.Minute-40*time.Second), 0)
	appendRecord(t, store, "expired", "UCexpired1234567890123", now.Add(-time.Hour), 600)
	appendRecord(t, store, "pending", "UCpending1234567890123", time.Time{}, 600)

	svc := adminservice.NewMonitorService(adminservice.MonitorServiceOptions{
		StreamersStore:      store,
		DefaultLeaseSeconds: 600,
		RenewWindow:         0.05,
		Now: func() time.Time {
			return now
		},
	})

	overview, err := svc.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview returned error: %v", err)
	}

	if overview.Summary.Total != 4 {
		t.Fatalf("expected 4 records, got %d", overview.Summary.Total)
	}
	if overview.Summary.Healthy != 1 || overview.Summary.Renewing != 1 || overview.Summary.Expired != 1 || overview.Summary.Pending != 1 {
		t.Fatalf("unexpected summary counts: %+v", overview.Summary)
	}

	healthy := findRecord(t, overview.Records, "healthy")
	if healthy.Status != adminservice.LeaseStatusHealthy {
		t.Fatalf("expected healthy status, got %s", healthy.Status)
	}
	if healthy.LeaseStart == nil || healthy.RenewAt == nil || healthy.LeaseExpires == nil {
		t.Fatalf("expected healthy record to include lease timestamps")
	}

	renewing := findRecord(t, overview.Records, "renewing")
	if renewing.Status != adminservice.LeaseStatusRenewing {
		t.Fatalf("expected renewing status, got %s", renewing.Status)
	}
	if renewing.LeaseSeconds != 600 {
		t.Fatalf("expected renewing leaseSeconds fallback to default, got %d", renewing.LeaseSeconds)
	}

	expired := findRecord(t, overview.Records, "expired")
	if expired.Status != adminservice.LeaseStatusExpired {
		t.Fatalf("expected expired status, got %s", expired.Status)
	}

	pending := findRecord(t, overview.Records, "pending")
	if pending.Status != adminservice.LeaseStatusPending {
		t.Fatalf("expected pending status, got %s", pending.Status)
	}
	if len(pending.Issues) == 0 {
		t.Fatalf("expected pending record to list issues")
	}
}

func appendRecord(t *testing.T, store *streamers.Store, alias, channelID string, leaseStart time.Time, leaseSeconds int) {
	t.Helper()
	record := streamers.Record{
		Streamer: streamers.Streamer{
			ID:    alias,
			Alias: alias,
		},
		Platforms: streamers.Platforms{
			YouTube: &streamers.YouTubePlatform{
				Handle:       "@" + alias,
				ChannelID:    channelID,
				HubLeaseDate: leaseStart.UTC().Format(time.RFC3339),
				LeaseSeconds: leaseSeconds,
				HubURL:       "https://pubsubhubbub.appspot.com/subscribe",
				CallbackURL:  "https://example.com/alerts",
			},
		},
	}
	if leaseStart.IsZero() {
		record.Platforms.YouTube.HubLeaseDate = ""
	}
	if _, err := store.Append(record); err != nil {
		t.Fatalf("append record %s: %v", alias, err)
	}
}

func findRecord(t *testing.T, records []adminservice.YouTubeLeaseRecord, alias string) adminservice.YouTubeLeaseRecord {
	t.Helper()
	for _, record := range records {
		if record.Alias == alias {
			return record
		}
	}
	t.Fatalf("record %s not found", alias)
	return adminservice.YouTubeLeaseRecord{}
}
