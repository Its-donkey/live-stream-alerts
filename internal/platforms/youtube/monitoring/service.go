package monitoring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"live-stream-alerts/internal/streamers"
)

const defaultRenewWindow = 0.05

// ServiceOptions configures the YouTube lease overview service.
type ServiceOptions struct {
	StreamersStore      *streamers.Store
	DefaultLeaseSeconds int
	RenewWindow         float64
	Now                 func() time.Time
}

// LeaseStatus describes the current renewal state for a YouTube subscription.
type LeaseStatus string

const (
	// LeaseStatusHealthy indicates the record is outside the renewal window.
	LeaseStatusHealthy LeaseStatus = "healthy"
	// LeaseStatusRenewing means the lease has reached the renewal window but has not expired.
	LeaseStatusRenewing LeaseStatus = "renewing"
	// LeaseStatusExpired signals the lease window has elapsed.
	LeaseStatusExpired LeaseStatus = "expired"
	// LeaseStatusPending represents records missing enough data to evaluate.
	LeaseStatusPending LeaseStatus = "pending"
)

// Overview summarises the lease status for every stored YouTube channel.
type Overview struct {
	Summary Summary      `json:"summary"`
	Records []LeaseEntry `json:"records"`
}

// Summary aggregates counts for the various lease states.
type Summary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Renewing int `json:"renewing"`
	Expired  int `json:"expired"`
	Pending  int `json:"pending"`
}

// LeaseEntry captures hub lease metadata for a streamer.
type LeaseEntry struct {
	StreamerID         string      `json:"streamerId"`
	Alias              string      `json:"alias"`
	ChannelID          string      `json:"channelId"`
	Handle             string      `json:"handle,omitempty"`
	HubURL             string      `json:"hubUrl,omitempty"`
	CallbackURL        string      `json:"callbackUrl,omitempty"`
	LeaseSeconds       int         `json:"leaseSeconds,omitempty"`
	LeaseStart         *time.Time  `json:"leaseStart,omitempty"`
	LeaseExpires       *time.Time  `json:"leaseExpires,omitempty"`
	RenewAt            *time.Time  `json:"renewAt,omitempty"`
	RenewWindowSeconds int         `json:"renewWindowSeconds,omitempty"`
	Status             LeaseStatus `json:"status"`
	Issues             []string    `json:"issues,omitempty"`
}

// Service exposes lease overview data for admin endpoints.
type Service struct {
	store               *streamers.Store
	defaultLeaseSeconds int
	renewWindow         float64
	now                 func() time.Time
}

// NewService constructs a Service from the provided options.
func NewService(opts ServiceOptions) *Service {
	store := opts.StreamersStore
	if store == nil {
		store = streamers.NewStore(streamers.DefaultFilePath)
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	renewWindow := opts.RenewWindow
	if renewWindow <= 0 || renewWindow >= 1 {
		renewWindow = defaultRenewWindow
	}
	return &Service{
		store:               store,
		defaultLeaseSeconds: opts.DefaultLeaseSeconds,
		renewWindow:         renewWindow,
		now:                 nowFn,
	}
}

// Overview returns the lease status for every streamer with YouTube metadata.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	var zero Overview
	if s == nil {
		return zero, errors.New("monitoring service is nil")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
	}
	if s.store == nil {
		return zero, errors.New("streamers store is not configured")
	}
	records, err := s.store.List()
	if err != nil {
		return zero, err
	}

	now := s.now().UTC()
	var result Overview
	for _, record := range records {
		entry := s.inspectRecord(record, now)
		if entry == nil {
			continue
		}
		result.Records = append(result.Records, *entry)
		result.Summary.Total++
		switch entry.Status {
		case LeaseStatusHealthy:
			result.Summary.Healthy++
		case LeaseStatusRenewing:
			result.Summary.Renewing++
		case LeaseStatusExpired:
			result.Summary.Expired++
		case LeaseStatusPending:
			result.Summary.Pending++
		}
	}
	return result, nil
}

func (s *Service) inspectRecord(record streamers.Record, now time.Time) *LeaseEntry {
	yt := record.Platforms.YouTube
	if yt == nil {
		return nil
	}
	entry := LeaseEntry{
		StreamerID:  record.Streamer.ID,
		Alias:       strings.TrimSpace(record.Streamer.Alias),
		ChannelID:   strings.TrimSpace(yt.ChannelID),
		Handle:      strings.TrimSpace(yt.Handle),
		HubURL:      strings.TrimSpace(yt.HubURL),
		CallbackURL: strings.TrimSpace(yt.CallbackURL),
		Status:      LeaseStatusPending,
	}
	if entry.ChannelID == "" {
		entry.Issues = append(entry.Issues, "channelId missing")
	}
	leaseSeconds := yt.LeaseSeconds
	if leaseSeconds <= 0 {
		leaseSeconds = s.defaultLeaseSeconds
	}
	if leaseSeconds > 0 {
		entry.LeaseSeconds = leaseSeconds
	} else {
		entry.Issues = append(entry.Issues, "leaseSeconds missing")
	}

	leaseStartStr := strings.TrimSpace(yt.HubLeaseDate)
	var leaseStart time.Time
	if leaseStartStr == "" {
		entry.Issues = append(entry.Issues, "hubLeaseDate missing")
	} else {
		parsed, err := time.Parse(time.RFC3339, leaseStartStr)
		if err != nil {
			entry.Issues = append(entry.Issues, fmt.Sprintf("hubLeaseDate invalid: %v", err))
		} else {
			leaseStart = parsed.UTC()
			entry.LeaseStart = &leaseStart
		}
	}

	if leaseSeconds <= 0 || leaseStart.IsZero() {
		return &entry
	}

	leaseDuration := time.Duration(leaseSeconds) * time.Second
	margin := s.renewWindowDuration(leaseDuration)
	renewAt := leaseStart.Add(leaseDuration - margin)
	expires := leaseStart.Add(leaseDuration)
	entry.RenewWindowSeconds = int(margin.Seconds())
	entry.LeaseExpires = &expires
	entry.RenewAt = &renewAt

	switch {
	case !now.Before(expires):
		entry.Status = LeaseStatusExpired
	case !now.Before(renewAt):
		entry.Status = LeaseStatusRenewing
	default:
		entry.Status = LeaseStatusHealthy
	}
	return &entry
}

func (s *Service) renewWindowDuration(leaseDuration time.Duration) time.Duration {
	margin := time.Duration(float64(leaseDuration) * s.renewWindow)
	if margin <= 0 || margin >= leaseDuration {
		margin = leaseDuration / 20
		if margin <= 0 {
			margin = time.Second
		}
	}
	return margin
}
