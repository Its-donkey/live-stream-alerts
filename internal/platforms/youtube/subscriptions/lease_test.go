package subscriptions

import (
	"path/filepath"
	"testing"
	"time"

	"live-stream-alerts/internal/streamers"
)

func TestRecordLeaseUpdatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	store := streamers.NewStore(path)
	record := streamers.Record{
		Streamer: streamers.Streamer{
			Alias:     "Example",
			FirstName: "Ex",
			LastName:  "Ample",
			Email:     "example@example.com",
		},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{ChannelID: "UC555", Handle: "@example"}},
	}
	if _, err := store.Append(record); err != nil {
		t.Fatalf("append: %v", err)
	}

	verifiedAt := time.Now().UTC()
	if err := RecordLease(store, "UC555", verifiedAt); err != nil {
		t.Fatalf("record lease: %v", err)
	}

	records, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if records[0].Platforms.YouTube.HubLeaseDate == "" {
		t.Fatalf("expected lease timestamp to be stored")
	}
}

func TestRecordLeaseValidatesInput(t *testing.T) {
	if err := RecordLease(nil, "", time.Now()); err == nil {
		t.Fatalf("expected error when channel ID missing")
	}
}
