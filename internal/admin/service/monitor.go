package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"live-stream-alerts/internal/streamers"
)

const defaultMonitorRenewWindow = 0.05

// MonitorServiceOptions configures the YouTube lease monitor service.
type MonitorServiceOptions struct {
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

// YouTubeMonitorOverview summarises the lease status for every stored YouTube channel.
type YouTubeMonitorOverview struct {
	Summary YouTubeMonitorSummary `json:"summary"`
	Records []YouTubeLeaseRecord  `json:"records"`
}

// YouTubeMonitorSummary aggregates counts for the various lease states.
type YouTubeMonitorSummary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Renewing int `json:"renewing"`
	Expired  int `json:"expired"`
	Pending  int `json:"pending"`
}

// YouTubeLeaseRecord captures hub lease metadata for a streamer.
type YouTubeLeaseRecord struct {
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

// MonitorService exposes high-level lease monitor state for admin endpoints.
type MonitorService struct {
	store               *streamers.Store
	defaultLeaseSeconds int
	renewWindow         float64
	now                 func() time.Time
}

// NewMonitorService constructs a MonitorService from the provided options.
func NewMonitorService(opts MonitorServiceOptions) *MonitorService {
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
		renewWindow = defaultMonitorRenewWindow
	}
	return &MonitorService{
		store:               store,
		defaultLeaseSeconds: opts.DefaultLeaseSeconds,
		renewWindow:         renewWindow,
		now:                 nowFn,
	}
}

// Overview returns the lease status for every streamer with YouTube metadata.
func (s *MonitorService) Overview(ctx context.Context) (YouTubeMonitorOverview, error) {
	var zero YouTubeMonitorOverview
	if s == nil {
		return zero, errors.New("monitor service is nil")
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
	var overview YouTubeMonitorOverview
	for _, record := range records {
		entry := s.inspectRecord(record, now)
		if entry == nil {
			continue
		}
		overview.Records = append(overview.Records, *entry)
		overview.Summary.Total++
		switch entry.Status {
		case LeaseStatusHealthy:
			overview.Summary.Healthy++
		case LeaseStatusRenewing:
			overview.Summary.Renewing++
		case LeaseStatusExpired:
			overview.Summary.Expired++
		case LeaseStatusPending:
			overview.Summary.Pending++
		}
	}
	return overview, nil
}

func (s *MonitorService) inspectRecord(record streamers.Record, now time.Time) *YouTubeLeaseRecord {
	yt := record.Platforms.YouTube
	if yt == nil {
		return nil
	}
	entry := YouTubeLeaseRecord{
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

func (s *MonitorService) renewWindowDuration(leaseDuration time.Duration) time.Duration {
	margin := time.Duration(float64(leaseDuration) * s.renewWindow)
	if margin <= 0 || margin >= leaseDuration {
		margin = leaseDuration / 20
		if margin <= 0 {
			margin = time.Second
		}
	}
	return margin
}
