package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/config"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/onboarding"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

// Action represents the allowed admin submission actions.
type Action string

const (
	// ActionApprove represents approving a pending submission.
	ActionApprove Action = "approve"
	// ActionReject represents rejecting a pending submission.
	ActionReject Action = "reject"
)

// ActionRequest captures the payload required to mutate a submission.
type ActionRequest struct {
	Action Action `json:"action"`
	ID     string `json:"id"`
}

// ActionResult contains the final status for the processed submission.
type ActionResult struct {
	Status     Action                 `json:"status"`
	Submission submissions.Submission `json:"submission"`
}

// Onboarder abstracts the YouTube onboarding workflow for dependency injection.
type Onboarder interface {
	FromURL(ctx context.Context, record streamers.Record, url string) error
}

// OnboarderFunc adapts a function to the Onboarder interface.
type OnboarderFunc func(context.Context, streamers.Record, string) error

// FromURL implements Onboarder.
func (f OnboarderFunc) FromURL(ctx context.Context, record streamers.Record, url string) error {
	if f == nil {
		return nil
	}
	return f(ctx, record, url)
}

// SubmissionsOptions configures the SubmissionsService.
type SubmissionsOptions struct {
	SubmissionsStore *submissions.Store
	StreamersStore   *streamers.Store
	YouTubeClient    *http.Client
	YouTube          config.YouTubeConfig
	Logger           logging.Logger
	Onboarder        Onboarder
}

// SubmissionsService encapsulates streamer submission review logic.
type SubmissionsService struct {
	submissionsStore *submissions.Store
	streamersStore   *streamers.Store
	youtubeClient    *http.Client
	youtube          config.YouTubeConfig
	logger           logging.Logger
	onboarder        Onboarder
}

// NewSubmissionsService constructs a SubmissionsService with the provided options.
func NewSubmissionsService(opts SubmissionsOptions) *SubmissionsService {
	submissionsStore := opts.SubmissionsStore
	if submissionsStore == nil {
		submissionsStore = submissions.NewStore(submissions.DefaultFilePath)
	}
	streamersStore := opts.StreamersStore
	if streamersStore == nil {
		streamersStore = streamers.NewStore(streamers.DefaultFilePath)
	}
	client := opts.YouTubeClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	svc := &SubmissionsService{
		submissionsStore: submissionsStore,
		streamersStore:   streamersStore,
		youtubeClient:    client,
		youtube:          opts.YouTube,
		logger:           opts.Logger,
		onboarder:        opts.Onboarder,
	}
	if svc.onboarder == nil {
		svc.onboarder = OnboarderFunc(func(ctx context.Context, record streamers.Record, url string) error {
			onboardOpts := onboarding.Options{
				Client:       svc.youtubeClient,
				HubURL:       strings.TrimSpace(svc.youtube.HubURL),
				CallbackURL:  strings.TrimSpace(svc.youtube.CallbackURL),
				VerifyMode:   strings.TrimSpace(svc.youtube.Verify),
				LeaseSeconds: svc.youtube.LeaseSeconds,
				Logger:       svc.logger,
				Store:        svc.streamersStore,
			}
			return onboarding.FromURL(ctx, record, url, onboardOpts)
		})
	}
	return svc
}

// List returns every pending submission.
func (s *SubmissionsService) List(ctx context.Context) ([]submissions.Submission, error) {
	if err := s.ensureStores(); err != nil {
		return nil, err
	}
	return s.submissionsStore.List()
}

// Process mutates a submission according to the provided action.
func (s *SubmissionsService) Process(ctx context.Context, req ActionRequest) (ActionResult, error) {
	if err := s.ensureStores(); err != nil {
		return ActionResult{}, err
	}
	action := normaliseAction(req.Action)
	if action == "" {
		return ActionResult{}, ErrInvalidAction
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return ActionResult{}, ErrMissingIdentifier
	}
	removed, err := s.submissionsStore.Remove(id)
	if err != nil {
		return ActionResult{}, err
	}
	if action == ActionApprove {
		if err := s.approve(ctx, removed); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Status: ActionApprove, Submission: removed}, nil
	}
	return ActionResult{Status: ActionReject, Submission: removed}, nil
}

func (s *SubmissionsService) ensureStores() error {
	if s == nil {
		return errors.New("submissions service is nil")
	}
	if s.submissionsStore == nil {
		return errors.New("submissions store is not configured")
	}
	if s.streamersStore == nil {
		return errors.New("streamers store is not configured")
	}
	return nil
}

func (s *SubmissionsService) approve(ctx context.Context, submission submissions.Submission) error {
	record := streamers.Record{
		Streamer: streamers.Streamer{
			ID:          streamers.GenerateID(),
			Alias:       submission.Alias,
			Description: submission.Description,
			Languages:   submission.Languages,
		},
	}
	persisted, err := s.streamersStore.Append(record)
	if err != nil {
		s.requeue(submission)
		return err
	}
	url := strings.TrimSpace(submission.PlatformURL)
	if url == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.onboarder.FromURL(ctx, persisted, url); err != nil && s.logger != nil {
		s.logger.Printf("failed to process platform url for %s: %v", persisted.Streamer.Alias, err)
	}
	return nil
}

func (s *SubmissionsService) requeue(submission submissions.Submission) {
	if _, err := s.submissionsStore.Append(submission); err != nil && s.logger != nil {
		s.logger.Printf("failed to requeue submission %s: %v", submission.ID, err)
	}
}

func normaliseAction(value Action) Action {
	normalized := Action(strings.ToLower(strings.TrimSpace(string(value))))
	switch normalized {
	case ActionApprove, ActionReject:
		return normalized
	default:
		return ""
	}
}

var (
	// ErrInvalidAction indicates the request payload contained an unsupported action.
	ErrInvalidAction = errors.New("action must be approve or reject")
	// ErrMissingIdentifier signals that the submission ID was omitted.
	ErrMissingIdentifier = errors.New("submission id is required")
)
