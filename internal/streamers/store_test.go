package streamers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")

	record := Record{
		Streamer: Streamer{
			Alias:     "Test",
			FirstName: "Test",
			LastName:  "User",
			Email:     "test@example.com",
		},
	}
	appended, err := Append(path, record)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if appended.Streamer.ID == "" {
		t.Fatalf("expected id to be generated")
	}

	list, err := List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one record")
	}
	list[0].Streamer.Alias = "mutated"
	again, err := List(path)
	if err != nil {
		t.Fatalf("list again: %v", err)
	}
	if again[0].Streamer.Alias != "Test" {
		t.Fatalf("list should return copy, got %q", again[0].Streamer.Alias)
	}
}

func TestAppendDuplicateID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")

	record := Record{Streamer: Streamer{ID: "fixed", Alias: "One", FirstName: "One", LastName: "One", Email: "one@example.com"}}
	if _, err := Append(path, record); err != nil {
		t.Fatalf("append: %v", err)
	}
	_, err := Append(path, record)
	if !errors.Is(err, ErrDuplicateStreamerID) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestAppendDuplicateAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")

	first := Record{Streamer: Streamer{Alias: "Edge Crafter"}}
	if _, err := Append(path, first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	second := Record{Streamer: Streamer{Alias: "EdgeCrafter!!"}}
	if _, err := Append(path, second); !errors.Is(err, ErrDuplicateAlias) {
		t.Fatalf("expected duplicate alias error, got %v", err)
	}
}

func TestUpdateYouTubeLiveStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	_, err := Append(path, Record{
		Streamer: Streamer{Alias: "Edge Crafter"},
		Platforms: Platforms{
			YouTube: &YouTubePlatform{ChannelID: "UCdemo"},
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	started := time.Date(2025, 11, 16, 9, 2, 41, 0, time.UTC)
	updated, err := UpdateYouTubeLiveStatus(path, "UCdemo", YouTubeLiveStatus{
		Live:      true,
		VideoID:   "video123",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("update live: %v", err)
	}
	if !updated.Status.Live || updated.Status.YouTube == nil || !updated.Status.YouTube.Live {
		t.Fatalf("expected live status to be true")
	}
	if updated.Status.YouTube.VideoID != "video123" {
		t.Fatalf("unexpected video id %q", updated.Status.YouTube.VideoID)
	}
	if !updated.Status.YouTube.StartedAt.Equal(started) {
		t.Fatalf("expected start time %v got %v", started, updated.Status.YouTube.StartedAt)
	}

	updated, err = UpdateYouTubeLiveStatus(path, "UCdemo", YouTubeLiveStatus{Live: false})
	if err != nil {
		t.Fatalf("update offline: %v", err)
	}
	if updated.Status.Live || (updated.Status.YouTube != nil && updated.Status.YouTube.Live) {
		t.Fatalf("expected live status to be cleared")
	}
	if updated.Status.YouTube != nil && updated.Status.YouTube.VideoID != "" {
		t.Fatalf("expected video id cleared, got %q", updated.Status.YouTube.VideoID)
	}

	if _, err := UpdateYouTubeLiveStatus(path, "missing", YouTubeLiveStatus{Live: true}); !errors.Is(err, ErrStreamerNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestUpdateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")

	record := Record{Streamer: Streamer{Alias: "Test", FirstName: "First", LastName: "Last", Email: "test@example.com"}}
	if _, err := Append(path, record); err != nil {
		t.Fatalf("append: %v", err)
	}

	err := UpdateFile(path, func(file *File) error {
		file.Records = append(file.Records, Record{Streamer: Streamer{Alias: "Two", FirstName: "Two", LastName: "Two", Email: "two@example.com"}})
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	list, err := List(path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected two records, got %d", len(list))
	}
}

func TestReadFileInitialisesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	data, err := readFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if data.SchemaRef == "" {
		t.Fatalf("expected schema to be initialised")
	}
}

func TestUpdateFileValidatesInputs(t *testing.T) {
	if err := UpdateFile("", nil); err == nil {
		t.Fatalf("expected error for missing path")
	}
	if err := UpdateFile("path", nil); err == nil {
		t.Fatalf("expected error for nil update function")
	}
}

func TestAppendValidatesPath(t *testing.T) {
	var store *Store
	if _, err := store.Append(Record{}); err == nil {
		t.Fatalf("expected error for nil store")
	}
}

func TestListValidatesPath(t *testing.T) {
	var store *Store
	if _, err := store.List(); err == nil {
		t.Fatalf("expected error for nil store")
	}
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	record, err := Append(path, Record{
		Streamer: Streamer{
			ID:        "OriginalID",
			Alias:     "Original",
			FirstName: "First",
			LastName:  "Last",
			Email:     "user@example.com",
		},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	newAlias := "Updated Alias"
	desc := "Updated description"
	langs := []string{"English", "Japanese"}

	updated, err := Update(path, UpdateFields{
		StreamerID:  record.Streamer.ID,
		Alias:       &newAlias,
		Description: &desc,
		Languages:   &langs,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Streamer.Alias != newAlias {
		t.Fatalf("alias not updated, got %q", updated.Streamer.Alias)
	}
	if updated.Streamer.Description != desc {
		t.Fatalf("description not updated")
	}
	if len(updated.Streamer.Languages) != len(langs) {
		t.Fatalf("languages not updated")
	}
	if updated.UpdatedAt.Equal(record.UpdatedAt) {
		t.Fatalf("expected updatedAt to change")
	}

	_, err = Update(path, UpdateFields{StreamerID: "missing", Alias: &newAlias})
	if !errors.Is(err, ErrStreamerNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
	if _, err := Update("", UpdateFields{StreamerID: "id", Alias: &newAlias}); err == nil {
		t.Fatalf("expected path validation error")
	}
	if _, err := Update(path, UpdateFields{}); err == nil {
		t.Fatalf("expected error for missing fields and id")
	}
	if _, err := Update(path, UpdateFields{StreamerID: "id"}); err == nil {
		t.Fatalf("expected error for no fields to update")
	}
}

func TestReadFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := readFile(path); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestSetYouTubeLiveUpdatesStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	if _, err := Append(path, Record{
		Streamer:  Streamer{Alias: "Test"},
		Platforms: Platforms{YouTube: &YouTubePlatform{ChannelID: "UC123"}},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	started := time.Now().UTC().Truncate(time.Second)
	updated, err := SetYouTubeLive(path, "UC123", "video1", started)
	if err != nil {
		t.Fatalf("set live: %v", err)
	}
	if updated.Status == nil || updated.Status.YouTube == nil || !updated.Status.YouTube.Live {
		t.Fatalf("expected youtube status to be live: %+v", updated.Status)
	}
	if updated.Status.YouTube.VideoID != "video1" {
		t.Fatalf("expected video id to be recorded, got %q", updated.Status.YouTube.VideoID)
	}
	if updated.Status.YouTube.StartedAt.IsZero() {
		t.Fatalf("expected start time to be set")
	}
	if !updated.Status.Live {
		t.Fatalf("expected overall status live")
	}
	if len(updated.Status.Platforms) != 1 || updated.Status.Platforms[0] != "youtube" {
		t.Fatalf("expected platforms to include youtube, got %v", updated.Status.Platforms)
	}
}

func TestClearYouTubeLiveResetsStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	if _, err := Append(path, Record{
		Streamer:  Streamer{Alias: "Test"},
		Platforms: Platforms{YouTube: &YouTubePlatform{ChannelID: "UC555"}},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := SetYouTubeLive(path, "UC555", "video-live", time.Now()); err != nil {
		t.Fatalf("set live: %v", err)
	}
	updated, err := ClearYouTubeLive(path, "UC555")
	if err != nil {
		t.Fatalf("clear live: %v", err)
	}
	if updated.Status == nil || updated.Status.YouTube == nil {
		t.Fatalf("expected youtube status struct")
	}
	if updated.Status.YouTube.Live {
		t.Fatalf("expected youtube live=false")
	}
	if updated.Status.YouTube.VideoID != "" {
		t.Fatalf("expected video id cleared, got %q", updated.Status.YouTube.VideoID)
	}
	if updated.Status.Live {
		t.Fatalf("expected overall status to be offline")
	}
	if len(updated.Status.Platforms) != 0 {
		t.Fatalf("expected platforms to be empty, got %v", updated.Status.Platforms)
	}
}
