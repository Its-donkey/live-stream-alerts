package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

func TestServiceCreateQueuesSubmission(t *testing.T) {
	dir := t.TempDir()
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	subStore := submissions.NewStore(filepath.Join(dir, "submissions.json"))
	svc := New(Options{Streamers: streamStore, Submissions: subStore})

	if _, err := svc.Create(t.Context(), CreateRequest{Alias: "Test", Description: "Example", Languages: []string{"English"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	items, err := subStore.List()
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(items) != 1 || items[0].Alias != "Test" {
		t.Fatalf("submission not stored: %+v", items)
	}
}

func TestServiceCreateRejectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	subStore := submissions.NewStore(filepath.Join(dir, "submissions.json"))
	if _, err := streamStore.Append(streamers.Record{Streamer: streamers.Streamer{Alias: "Test"}}); err != nil {
		t.Fatalf("append: %v", err)
	}
	svc := New(Options{Streamers: streamStore, Submissions: subStore})
	if _, err := svc.Create(t.Context(), CreateRequest{Alias: "Test"}); err == nil {
		t.Fatalf("expected duplicate error")
	}
}

func TestServiceUpdateValidatesInput(t *testing.T) {
	svc := New(Options{Streamers: streamers.NewStore(filepath.Join(t.TempDir(), "streamers.json")), Submissions: submissions.NewStore(filepath.Join(t.TempDir(), "subs.json"))})
	if _, err := svc.Update(t.Context(), UpdateRequest{}); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestServiceDeleteUnsubscribesAndRemoves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "streamers.json")
	streamStore := streamers.NewStore(path)
	if _, err := streamStore.Append(streamers.Record{
		Streamer: streamers.Streamer{ID: "to-delete", Alias: "ToDelete"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID:   "UC123",
			CallbackURL: "https://example.com/hook",
		}},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	subStore := submissions.NewStore(filepath.Join(dir, "subs.json"))
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("hub.mode") != "unsubscribe" {
			t.Fatalf("expected unsubscribe mode")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hub.Close()
	svc := New(Options{
		Streamers:     streamStore,
		Submissions:   subStore,
		YouTubeClient: hub.Client(),
		YouTubeHubURL: hub.URL,
	})
	if err := svc.Delete(t.Context(), DeleteRequest{ID: "to-delete"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	records, err := streamStore.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected record removed")
	}
}

func TestServiceDeleteSubscriptionFailure(t *testing.T) {
	dir := t.TempDir()
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	if _, err := streamStore.Append(streamers.Record{
		Streamer: streamers.Streamer{ID: "boom", Alias: "Boom"},
		Platforms: streamers.Platforms{YouTube: &streamers.YouTubePlatform{
			ChannelID:   "UC999",
			CallbackURL: "https://example.com/hook",
		}},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	svc := New(Options{
		Streamers:     streamStore,
		Submissions:   submissions.NewStore(filepath.Join(dir, "subs.json")),
		YouTubeClient: http.DefaultClient,
		YouTubeHubURL: "http://127.0.0.1:1", // force failure
	})
	if err := svc.Delete(t.Context(), DeleteRequest{ID: "boom"}); !errors.Is(err, ErrSubscription) {
		t.Fatalf("expected subscription error, got %v", err)
	}
}
