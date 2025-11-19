package service

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"live-stream-alerts/config"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

func TestSubmissionsServiceReject(t *testing.T) {
	dir := t.TempDir()
	subStore := submissions.NewStore(filepath.Join(dir, "subs.json"))
	if _, err := subStore.Append(submissions.Submission{ID: "sub_1", Alias: "Test", SubmittedAt: time.Now()}); err != nil {
		t.Fatalf("append submission: %v", err)
	}
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	svc := NewSubmissionsService(SubmissionsOptions{
		SubmissionsStore: subStore,
		StreamersStore:   streamStore,
	})
	result, err := svc.Process(context.Background(), ActionRequest{Action: ActionReject, ID: "sub_1"})
	if err != nil {
		t.Fatalf("process reject: %v", err)
	}
	if result.Status != ActionReject {
		t.Fatalf("expected reject status, got %s", result.Status)
	}
	remaining, err := subStore.List()
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected submissions cleared, got %d", len(remaining))
	}
}

func TestSubmissionsServiceApprove(t *testing.T) {
	dir := t.TempDir()
	subStore := submissions.NewStore(filepath.Join(dir, "subs.json"))
	pending, err := subStore.Append(submissions.Submission{
		ID:          "sub_1",
		Alias:       "Test",
		PlatformURL: "https://youtube.com/channel/abc",
		SubmittedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("append submission: %v", err)
	}
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	onboarder := &stubOnboarder{}
	svc := NewSubmissionsService(SubmissionsOptions{
		SubmissionsStore: subStore,
		StreamersStore:   streamStore,
		Onboarder:        onboarder,
		YouTubeClient:    http.DefaultClient,
		YouTube: config.YouTubeConfig{
			HubURL:       "https://hub.example.com",
			CallbackURL:  "https://callback.example.com",
			Verify:       "async",
			LeaseSeconds: 60,
		},
	})
	result, err := svc.Process(context.Background(), ActionRequest{Action: ActionApprove, ID: pending.ID})
	if err != nil {
		t.Fatalf("process approval: %v", err)
	}
	if result.Status != ActionApprove {
		t.Fatalf("expected approve status, got %s", result.Status)
	}
	if !onboarder.called {
		t.Fatalf("expected onboarding to be triggered")
	}
	records, err := streamStore.List()
	if err != nil {
		t.Fatalf("list streamers: %v", err)
	}
	if len(records) != 1 || records[0].Streamer.Alias != "Test" {
		t.Fatalf("expected streamer appended, got %+v", records)
	}
}

func TestSubmissionsServiceDuplicateAlias(t *testing.T) {
	dir := t.TempDir()
	subStore := submissions.NewStore(filepath.Join(dir, "subs.json"))
	if _, err := subStore.Append(submissions.Submission{ID: "sub_1", Alias: "Test", SubmittedAt: time.Now()}); err != nil {
		t.Fatalf("append submission: %v", err)
	}
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	if _, err := streamStore.Append(streamers.Record{Streamer: streamers.Streamer{Alias: "Test"}}); err != nil {
		t.Fatalf("seed streamers: %v", err)
	}
	svc := NewSubmissionsService(SubmissionsOptions{
		SubmissionsStore: subStore,
		StreamersStore:   streamStore,
	})
	if _, err := svc.Process(context.Background(), ActionRequest{Action: ActionApprove, ID: "sub_1"}); err == nil {
		t.Fatalf("expected duplicate alias error")
	}
	list, err := subStore.List()
	if err != nil {
		t.Fatalf("list submissions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected submission requeued, got %d", len(list))
	}
}

func TestSubmissionsServiceValidation(t *testing.T) {
	svc := NewSubmissionsService(SubmissionsOptions{
		SubmissionsStore: submissions.NewStore(filepath.Join(t.TempDir(), "subs.json")),
		StreamersStore:   streamers.NewStore(filepath.Join(t.TempDir(), "stream.json")),
	})
	if _, err := svc.Process(context.Background(), ActionRequest{Action: "invalid", ID: "1"}); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected invalid action error, got %v", err)
	}
	if _, err := svc.Process(context.Background(), ActionRequest{Action: ActionApprove}); !errors.Is(err, ErrMissingIdentifier) {
		t.Fatalf("expected missing id error, got %v", err)
	}
	if _, err := svc.Process(context.Background(), ActionRequest{Action: ActionReject, ID: "nope"}); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestSubmissionsServiceIgnoresOnboardingErrors(t *testing.T) {
	dir := t.TempDir()
	subStore := submissions.NewStore(filepath.Join(dir, "subs.json"))
	if _, err := subStore.Append(submissions.Submission{ID: "sub_1", Alias: "Test", PlatformURL: "https://youtube.com", SubmittedAt: time.Now()}); err != nil {
		t.Fatalf("append submission: %v", err)
	}
	streamStore := streamers.NewStore(filepath.Join(dir, "streamers.json"))
	onboarder := &stubOnboarder{err: errors.New("boom")}
	svc := NewSubmissionsService(SubmissionsOptions{
		SubmissionsStore: subStore,
		StreamersStore:   streamStore,
		Onboarder:        onboarder,
	})
	if _, err := svc.Process(context.Background(), ActionRequest{Action: ActionApprove, ID: "sub_1"}); err != nil {
		t.Fatalf("expected approval to succeed despite onboarding error: %v", err)
	}
}

type stubOnboarder struct {
	called bool
	err    error
}

func (s *stubOnboarder) FromURL(ctx context.Context, record streamers.Record, url string) error {
	s.called = true
	return s.err
}
