package submissions_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"live-stream-alerts/internal/submissions"
)

func TestStoreAppendPopulatesDefaults(t *testing.T) {
	dir := t.TempDir()
	fixed := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	store := submissions.NewStore(
		filepath.Join(dir, "subs.json"),
		submissions.WithNow(func() time.Time { return fixed }),
	)

	saved, err := store.Append(submissions.Submission{Alias: "Example"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if saved.ID == "" {
		t.Fatalf("expected ID to be generated")
	}
	if !saved.SubmittedAt.Equal(fixed) {
		t.Fatalf("expected SubmittedAt to use injected clock, got %s", saved.SubmittedAt)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Alias != "Example" {
		t.Fatalf("unexpected list contents: %+v", list)
	}

	// ensure list returns a copy
	list[0].Alias = "Mutated"
	list2, _ := store.List()
	if list2[0].Alias != "Example" {
		t.Fatalf("list mutated original slice")
	}
}

func TestStoreAppendRespectsProvidedFields(t *testing.T) {
	dir := t.TempDir()
	store := submissions.NewStore(filepath.Join(dir, "subs.json"))
	now := time.Now().Add(-time.Hour).UTC()
	input := submissions.Submission{ID: "sub_1", Alias: "Preset", SubmittedAt: now}
	saved, err := store.Append(input)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if saved.ID != "sub_1" {
		t.Fatalf("expected ID preserved, got %s", saved.ID)
	}
	if !saved.SubmittedAt.Equal(now) {
		t.Fatalf("expected SubmittedAt preserved, got %s", saved.SubmittedAt)
	}
}

func TestStoreRemove(t *testing.T) {
	dir := t.TempDir()
	store := submissions.NewStore(filepath.Join(dir, "subs.json"))
	first, _ := store.Append(submissions.Submission{Alias: "One"})
	second, _ := store.Append(submissions.Submission{Alias: "Two"})

	t.Run("removes existing entry", func(t *testing.T) {
		removed, err := store.Remove(first.ID)
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		if removed.ID != first.ID {
			t.Fatalf("expected removed %s, got %s", first.ID, removed.ID)
		}
		list, _ := store.List()
		if len(list) != 1 || list[0].ID != second.ID {
			t.Fatalf("expected only second entry to remain, got %+v", list)
		}
	})

	t.Run("missing entry returns ErrNotFound", func(t *testing.T) {
		if _, err := store.Remove("does-not-exist"); err != submissions.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestNewStoreCreatesDefaultPathWhenEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := submissions.NewStore("")
	if store.Path() != submissions.DefaultFilePath {
		t.Fatalf("expected default path, got %s", store.Path())
	}

	// Ensure append creates directories when necessary.
	defer func() { _ = os.Remove(store.Path()) }()
	if _, err := store.Append(submissions.Submission{Alias: "Test"}); err != nil {
		t.Fatalf("append: %v", err)
	}
}

func TestStoreUsesInjectedIDGenerator(t *testing.T) {
	dir := t.TempDir()
	store := submissions.NewStore(
		filepath.Join(dir, "subs.json"),
		submissions.WithIDGenerator(func() string { return "fixed-id" }),
	)
	saved, err := store.Append(submissions.Submission{Alias: "Example"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if saved.ID != "fixed-id" {
		t.Fatalf("expected injected ID generator to run, got %s", saved.ID)
	}
}
