package streamers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
	if _, err := Append("", Record{}); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestListValidatesPath(t *testing.T) {
	if _, err := List(""); err == nil {
		t.Fatalf("expected error for empty path")
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
