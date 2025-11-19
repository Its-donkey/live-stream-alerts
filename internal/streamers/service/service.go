package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

// ErrValidation indicates the request payload is invalid.
var ErrValidation = errors.New("validation error")

// ErrSubscription signals a downstream subscription/unsubscription failure.
var ErrSubscription = errors.New("subscription error")

// Options configures a Service instance.
type Options struct {
	Streamers     *streamers.Store
	Submissions   *submissions.Store
	YouTubeClient *http.Client
	YouTubeHubURL string
}

// Service implements the business logic for streamer operations.
type Service struct {
	streamers     *streamers.Store
	submissions   *submissions.Store
	youtubeClient *http.Client
	youtubeHubURL string
}

// CreateRequest captures the fields accepted by Create.
type CreateRequest struct {
	Alias       string
	Description string
	Languages   []string
	PlatformURL string
}

// CreateResult captures the stored submission returned by Create.
type CreateResult struct {
	Submission submissions.Submission
}

// UpdateRequest captures mutable streamer fields.
type UpdateRequest struct {
	ID          string
	Alias       *string
	Description *string
	Languages   *[]string
}

// DeleteRequest describes the streamer deletion payload.
type DeleteRequest struct {
	ID string
}

// New instantiates a Service.
func New(opts Options) *Service {
	return &Service{
		streamers:     opts.Streamers,
		submissions:   opts.Submissions,
		youtubeClient: opts.YouTubeClient,
		youtubeHubURL: strings.TrimSpace(opts.YouTubeHubURL),
	}
}

// List returns every stored streamer record.
func (s *Service) List(ctx context.Context) ([]streamers.Record, error) {
	if err := s.ensureStores(); err != nil {
		return nil, err
	}
	return s.streamers.List()
}

// Create enqueues a streamer submission after validating the payload.
func (s *Service) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if err := s.ensureStores(); err != nil {
		return CreateResult{}, err
	}
	alias := strings.TrimSpace(req.Alias)
	if alias == "" {
		return CreateResult{}, fmt.Errorf("%w: streamer.alias is required", ErrValidation)
	}
	if streamers.NormaliseAlias(alias) == "" {
		return CreateResult{}, fmt.Errorf("%w: streamer.alias must contain a letter or digit", ErrValidation)
	}
	langs, err := sanitiseLanguages(req.Languages)
	if err != nil {
		return CreateResult{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if err := s.ensureUniqueAlias(alias); err != nil {
		return CreateResult{}, err
	}
	submission := submissions.Submission{
		Alias:       alias,
		Description: strings.TrimSpace(req.Description),
		Languages:   langs,
		PlatformURL: strings.TrimSpace(req.PlatformURL),
	}
	saved, err := s.submissions.Append(submission)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Submission: saved}, nil
}

// Update modifies an existing streamer record.
func (s *Service) Update(ctx context.Context, req UpdateRequest) (streamers.Record, error) {
	if err := s.ensureStores(); err != nil {
		return streamers.Record{}, err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return streamers.Record{}, fmt.Errorf("%w: streamer.id is required", ErrValidation)
	}
	update := streamers.UpdateFields{StreamerID: id}
	var hasUpdate bool
	if req.Alias != nil {
		alias := strings.TrimSpace(*req.Alias)
		if alias == "" {
			return streamers.Record{}, fmt.Errorf("%w: streamer.alias cannot be blank", ErrValidation)
		}
		update.Alias = &alias
		hasUpdate = true
	}
	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		update.Description = &desc
		hasUpdate = true
	}
	if req.Languages != nil {
		langs, err := sanitiseLanguages(*req.Languages)
		if err != nil {
			return streamers.Record{}, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		update.Languages = new([]string)
		*update.Languages = langs
		hasUpdate = true
	}
	if !hasUpdate {
		return streamers.Record{}, fmt.Errorf("%w: at least one streamer field must be provided", ErrValidation)
	}
	return s.streamers.Update(update)
}

// Delete removes a streamer, unsubscribing from alerts when required.
func (s *Service) Delete(ctx context.Context, req DeleteRequest) error {
	if err := s.ensureStores(); err != nil {
		return err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return fmt.Errorf("%w: streamer.id is required", ErrValidation)
	}
	record, err := s.streamers.Get(id)
	if err != nil {
		return err
	}
	if record.Platforms.YouTube != nil {
		if err := s.unsubscribe(ctx, record); err != nil {
			return err
		}
	}
	return s.streamers.Delete(id)
}

func (s *Service) unsubscribe(ctx context.Context, record streamers.Record) error {
	client := s.youtubeClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	unsubOpts := subscriptions.Options{
		Client: client,
		HubURL: s.youtubeHubURL,
		Mode:   "unsubscribe",
	}
	if err := subscriptions.ManageSubscription(ctx, record, unsubOpts); err != nil {
		return fmt.Errorf("%w: %v", ErrSubscription, err)
	}
	return nil
}

func (s *Service) ensureStores() error {
	if s == nil {
		return errors.New("streamer service is nil")
	}
	if s.streamers == nil {
		return errors.New("streamers store is not configured")
	}
	if s.submissions == nil {
		return errors.New("submissions store is not configured")
	}
	return nil
}

func (s *Service) ensureUniqueAlias(alias string) error {
	key := streamers.NormaliseAlias(alias)
	if key == "" {
		return fmt.Errorf("%w: streamer.alias must contain at least one letter or digit", ErrValidation)
	}
	records, err := s.streamers.List()
	if err != nil {
		return err
	}
	for _, rec := range records {
		if key == streamers.NormaliseAlias(rec.Streamer.Alias) {
			return streamers.ErrDuplicateAlias
		}
	}
	pending, err := s.submissions.List()
	if err != nil {
		return err
	}
	for _, sub := range pending {
		if key == streamers.NormaliseAlias(sub.Alias) {
			return streamers.ErrDuplicateAlias
		}
	}
	return nil
}

func sanitiseLanguages(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	clean := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("streamer.languages contains an empty entry")
		}
		if _, ok := allowedLanguagesSet[trimmed]; !ok {
			return nil, fmt.Errorf("streamer.languages contains unsupported value %q", trimmed)
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		clean = append(clean, trimmed)
	}
	return clean, nil
}

var allowedLanguagesSet = func() map[string]struct{} {
	values := []string{
		"English", "Afrikaans", "Albanian", "Amharic", "Armenian", "Azerbaijani",
		"Basque", "Belarusian", "Bosnian", "Bulgarian", "Catalan", "Cebuano",
		"Croatian", "Czech", "Danish", "Dutch", "Estonian", "Filipino",
		"Finnish", "Galician", "Georgian", "German", "Greek", "Gujarati",
		"Haitian Creole", "Hebrew", "Hmong", "Hungarian", "Icelandic", "Igbo",
		"Italian", "Japanese", "Javanese", "Kannada", "Kazakh", "Khmer",
		"Kinyarwanda", "Korean", "Kurdish", "Lao", "Latvian", "Lithuanian",
		"Luxembourgish", "Macedonian", "Malay", "Malayalam", "Maltese", "Marathi",
		"Mongolian", "Nepali", "Norwegian", "Pashto", "Persian", "Polish",
		"Punjabi", "Romanian", "Serbian", "Sinhala", "Slovak", "Slovenian",
		"Somali", "Swahili", "Swedish", "Tamil", "Telugu", "Thai", "Turkish",
		"Ukrainian", "Urdu", "Uzbek", "Vietnamese", "Welsh", "Xhosa", "Yoruba", "Zulu",
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}()
