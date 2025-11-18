package streamers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultSchemaPath points to the JSON schema that describes the on-disk format.
	DefaultSchemaPath = "../schema/streamers.schema.json"
	// DefaultFilePath is the default location for storing streamer records.
	DefaultFilePath = "data/streamers.json"
)

// File represents the on-disk format containing all streamer records.
type File struct {
	SchemaRef string   `json:"$schema"`
	Records   []Record `json:"streamers"`
}

// Record matches the JSON schema for a single streamer entry.
type Record struct {
	Streamer  Streamer  `json:"streamer"`
	Platforms Platforms `json:"platforms"`
	Status    *Status   `json:"status,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Streamer captures personal information for a streamer.
type Streamer struct {
	ID          string   `json:"id"`
	Alias       string   `json:"alias"`
	Description string   `json:"description,omitempty"`
	FirstName   string   `json:"firstName"`
	LastName    string   `json:"lastName"`
	Email       string   `json:"email"`
	City        string   `json:"city,omitempty"`
	Country     string   `json:"country,omitempty"`
	Languages   []string `json:"languages,omitempty"`
}

// Platforms groups platform-specific configuration.
type Platforms struct {
	YouTube  *YouTubePlatform  `json:"youtube,omitempty"`
	Facebook *FacebookPlatform `json:"facebook,omitempty"`
	Twitch   *TwitchPlatform   `json:"twitch,omitempty"`
}

// Status tracks live information for each platform.
type Status struct {
	Live      bool            `json:"live"`
	Platforms []string        `json:"platforms,omitempty"`
	YouTube   *YouTubeStatus  `json:"youtube,omitempty"`
	Twitch    *TwitchStatus   `json:"twitch,omitempty"`
	Facebook  *FacebookStatus `json:"facebook,omitempty"`
}

// YouTubeStatus stores the current live metadata for YouTube.
type YouTubeStatus struct {
	Live      bool      `json:"live"`
	VideoID   string    `json:"videoId,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

// TwitchStatus stores Twitch live metadata.
type TwitchStatus struct {
	Live      bool      `json:"live"`
	StreamID  string    `json:"streamId,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

// FacebookStatus stores Facebook Live metadata.
type FacebookStatus struct {
	Live      bool      `json:"live"`
	VideoID   string    `json:"videoId,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

// YouTubePlatform stores YouTube-specific metadata.
type YouTubePlatform struct {
	Handle       string `json:"handle"`
	ChannelID    string `json:"channelId,omitempty"`
	HubSecret    string `json:"hubSecret,omitempty"`
	HubLeaseDate string `json:"hubLeaseDate,omitempty"`
	Topic        string `json:"topic,omitempty"`
	CallbackURL  string `json:"callbackUrl,omitempty"`
	HubURL       string `json:"hubUrl,omitempty"`
	VerifyMode   string `json:"verifyMode,omitempty"`
	LeaseSeconds int    `json:"leaseSeconds,omitempty"`
}

// FacebookPlatform stores Facebook-specific metadata.
type FacebookPlatform struct {
	PageID      string `json:"pageId,omitempty"`
	AccessToken string `json:"accessToken,omitempty"`
}

// TwitchPlatform stores Twitch-specific metadata.
type TwitchPlatform struct {
	Username      string `json:"username,omitempty"`
	BroadcasterID string `json:"broadcasterId,omitempty"`
}

var (
	fileMu                 sync.Mutex
	ErrDuplicateStreamerID = errors.New("streamer id already exists")
	ErrStreamerNotFound    = errors.New("streamer not found")
	ErrDuplicateAlias      = errors.New("streamer alias already exists")
)

// UpdateFields describes the mutable streamer fields.
type UpdateFields struct {
	StreamerID  string
	Alias       *string
	Description *string
	Languages   *[]string
}

// Append adds a new streamer record to disk and returns a copy with timestamps populated.
func Append(path string, record Record) (Record, error) {
	if path == "" {
		return Record{}, errors.New("streamers file path is required")
	}

	fileMu.Lock()
	defer fileMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Record{}, fmt.Errorf("create streamers dir: %w", err)
	}

	fileData, err := readFile(path)
	if err != nil {
		return Record{}, err
	}
	if fileData.SchemaRef == "" {
		fileData.SchemaRef = DefaultSchemaPath
	}

	if record.Streamer.ID == "" {
		record.Streamer.ID = GenerateID()
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	newAliasKey := NormaliseAlias(record.Streamer.Alias)
	for _, existing := range fileData.Records {
		if existing.Streamer.ID == record.Streamer.ID {
			return Record{}, fmt.Errorf("%w: %s", ErrDuplicateStreamerID, record.Streamer.ID)
		}
		if newAliasKey != "" && newAliasKey == NormaliseAlias(existing.Streamer.Alias) {
			return Record{}, fmt.Errorf("%w: %s", ErrDuplicateAlias, record.Streamer.Alias)
		}
	}

	fileData.Records = append(fileData.Records, record)

	encoded, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return Record{}, fmt.Errorf("encode streamers file: %w", err)
	}

	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return Record{}, fmt.Errorf("write streamers file: %w", err)
	}

	return record, nil
}

// List loads all streamer records from disk.
func List(path string) ([]Record, error) {
	if path == "" {
		return nil, errors.New("streamers file path is required")
	}

	fileMu.Lock()
	defer fileMu.Unlock()

	fileData, err := readFile(path)
	if err != nil {
		return nil, err
	}

	records := make([]Record, len(fileData.Records))
	copy(records, fileData.Records)
	return records, nil
}

// YouTubeLiveStatus describes the live state to persist for a YouTube channel.
type YouTubeLiveStatus struct {
	Live      bool
	VideoID   string
	StartedAt time.Time
}

// UpdateYouTubeLiveStatus updates the stored status for the streamer owning the channel ID.
func UpdateYouTubeLiveStatus(path, channelID string, liveStatus YouTubeLiveStatus) (Record, error) {
	if path == "" {
		return Record{}, errors.New("streamers file path is required")
	}
	ch := strings.TrimSpace(channelID)
	if ch == "" {
		return Record{}, errors.New("channel id is required")
	}

	var updated Record
	err := UpdateFile(path, func(file *File) error {
		for i := range file.Records {
			yt := file.Records[i].Platforms.YouTube
			if yt == nil || !strings.EqualFold(yt.ChannelID, ch) {
				continue
			}
			applyYouTubeStatus(&file.Records[i], liveStatus)
			file.Records[i].UpdatedAt = time.Now().UTC()
			updated = file.Records[i]
			return nil
		}
		return fmt.Errorf("%w: %s", ErrStreamerNotFound, ch)
	})
	if err != nil {
		return Record{}, err
	}
	return updated, nil
}

func applyYouTubeStatus(record *Record, liveStatus YouTubeLiveStatus) {
	if record.Status == nil {
		record.Status = &Status{}
	}
	if record.Status.YouTube == nil {
		record.Status.YouTube = &YouTubeStatus{}
	}
	record.Status.YouTube.Live = liveStatus.Live
	record.Status.YouTube.VideoID = liveStatus.VideoID
	if liveStatus.StartedAt.IsZero() {
		record.Status.YouTube.StartedAt = time.Time{}
	} else {
		record.Status.YouTube.StartedAt = liveStatus.StartedAt.UTC()
	}

	if liveStatus.Live {
		record.Status.Platforms = addPlatform(record.Status.Platforms, platformYouTube)
	} else {
		record.Status.Platforms = removePlatform(record.Status.Platforms, platformYouTube)
	}
	record.Status.Live = liveStatus.Live
	if !liveStatus.Live && record.Status.YouTube != nil {
		record.Status.YouTube.Live = false
		record.Status.YouTube.VideoID = ""
		record.Status.YouTube.StartedAt = time.Time{}
	}
	if len(record.Status.Platforms) == 0 && !record.Status.Live {
		record.Status.Platforms = nil
	}
}

func addPlatform(platforms []string, platform string) []string {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return platforms
	}
	for _, existing := range platforms {
		if strings.EqualFold(existing, platform) {
			return platforms
		}
	}
	return append(platforms, platform)
}

func removePlatform(platforms []string, platform string) []string {
	if len(platforms) == 0 {
		return platforms
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	out := platforms[:0]
	for _, existing := range platforms {
		if strings.EqualFold(existing, platform) {
			continue
		}
		out = append(out, existing)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Update applies modifications to an existing streamer.
func Update(path string, fields UpdateFields) (Record, error) {
	if path == "" {
		return Record{}, errors.New("streamers file path is required")
	}
	id := strings.TrimSpace(fields.StreamerID)
	if id == "" {
		return Record{}, errors.New("streamer id is required")
	}
	if fields.Alias == nil && fields.Description == nil && fields.Languages == nil {
		return Record{}, errors.New("no fields provided to update")
	}

	var updated Record
	err := UpdateFile(path, func(file *File) error {
		for i := range file.Records {
			if !strings.EqualFold(file.Records[i].Streamer.ID, id) {
				continue
			}
			if fields.Alias != nil {
				file.Records[i].Streamer.Alias = *fields.Alias
			}
			if fields.Description != nil {
				file.Records[i].Streamer.Description = *fields.Description
			}
			if fields.Languages != nil {
				file.Records[i].Streamer.Languages = append([]string(nil), (*fields.Languages)...)
			}
			file.Records[i].UpdatedAt = time.Now().UTC()
			updated = file.Records[i]
			return nil
		}
		return fmt.Errorf("%w: %s", ErrStreamerNotFound, id)
	})
	if err != nil {
		return Record{}, err
	}
	return updated, nil
}

// UpdateFile reads the streamers file, applies the provided mutation, and writes it back to disk atomically.
func UpdateFile(path string, updateFn func(*File) error) error {
	if path == "" {
		return errors.New("streamers file path is required")
	}
	if updateFn == nil {
		return errors.New("updateFn is required")
	}

	fileMu.Lock()
	defer fileMu.Unlock()

	fileData, err := readFile(path)
	if err != nil {
		return err
	}

	if err := updateFn(&fileData); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return fmt.Errorf("encode streamers file: %w", err)
	}

	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write streamers file: %w", err)
	}
	return nil
}

// Delete removes a streamer by ID.
func Delete(path, streamerID string) error {
	if path == "" {
		return errors.New("streamers file path is required")
	}
	streamerID = strings.TrimSpace(streamerID)
	if streamerID == "" {
		return errors.New("streamer id is required")
	}

	return UpdateFile(path, func(file *File) error {
		for i := range file.Records {
			if strings.EqualFold(file.Records[i].Streamer.ID, streamerID) {
				file.Records = append(file.Records[:i], file.Records[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrStreamerNotFound, streamerID)
	})
}

// Get returns a single streamer record by ID.
func Get(path, streamerID string) (Record, error) {
	if path == "" {
		return Record{}, errors.New("streamers file path is required")
	}
	streamerID = strings.TrimSpace(streamerID)
	if streamerID == "" {
		return Record{}, errors.New("streamer id is required")
	}

	fileMu.Lock()
	defer fileMu.Unlock()

	fileData, err := readFile(path)
	if err != nil {
		return Record{}, err
	}

	for _, record := range fileData.Records {
		if strings.EqualFold(record.Streamer.ID, streamerID) {
			return record, nil
		}
	}
	return Record{}, fmt.Errorf("%w: %s", ErrStreamerNotFound, streamerID)
}

// SetYouTubeLive marks the streamer associated with the provided channel ID as live.
func SetYouTubeLive(path, channelID, videoID string, startedAt time.Time) (Record, error) {
	return updateYouTubeStatus(path, channelID, func(status *Status) {
		if status.YouTube == nil {
			status.YouTube = &YouTubeStatus{}
		}
		status.YouTube.Live = true
		status.YouTube.VideoID = videoID
		if !startedAt.IsZero() {
			status.YouTube.StartedAt = startedAt
		} else {
			status.YouTube.StartedAt = time.Time{}
		}
		status.Platforms = addPlatform(status.Platforms, platformYouTube)
		status.Live = true
	})
}

// ClearYouTubeLive marks the YouTube platform as offline for the matching channel ID.
func ClearYouTubeLive(path, channelID string) (Record, error) {
	return updateYouTubeStatus(path, channelID, func(status *Status) {
		if status.YouTube == nil {
			status.YouTube = &YouTubeStatus{}
		}
		status.YouTube.Live = false
		status.YouTube.VideoID = ""
		status.YouTube.StartedAt = time.Time{}
		status.Platforms = removePlatform(status.Platforms, platformYouTube)
	})
}

const platformYouTube = "youtube"

func updateYouTubeStatus(path, channelID string, updateFn func(*Status)) (Record, error) {
	if path == "" {
		return Record{}, errors.New("streamers file path is required")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return Record{}, errors.New("youtube channel id is required")
	}
	var updated Record
	err := UpdateFile(path, func(file *File) error {
		for i := range file.Records {
			yt := file.Records[i].Platforms.YouTube
			if !channelMatches(yt, channelID) {
				continue
			}
			if file.Records[i].Status == nil {
				file.Records[i].Status = &Status{}
			}
			updateFn(file.Records[i].Status)
			refreshLiveFlag(file.Records[i].Status)
			file.Records[i].UpdatedAt = time.Now().UTC()
			updated = file.Records[i]
			return nil
		}
		return fmt.Errorf("%w: %s", ErrStreamerNotFound, channelID)
	})
	return updated, err
}

func channelMatches(yt *YouTubePlatform, target string) bool {
	if yt == nil {
		return false
	}
	stored := strings.TrimSpace(yt.ChannelID)
	if stored == "" {
		stored = extractChannelIDFromTopic(yt.Topic)
	}
	if stored == "" || target == "" {
		return false
	}
	if strings.EqualFold(stored, target) {
		return true
	}
	return strings.EqualFold(trimChannelPrefix(stored), trimChannelPrefix(target))
}

func trimChannelPrefix(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	return strings.TrimPrefix(value, "UC")
}

func extractChannelIDFromTopic(topic string) string {
	if topic == "" {
		return ""
	}
	u, err := url.Parse(topic)
	if err != nil {
		return ""
	}
	return u.Query().Get("channel_id")
}

func refreshLiveFlag(status *Status) {
	if status == nil {
		return
	}
	status.Live = len(status.Platforms) > 0
	if status.Live {
		return
	}
	if status.YouTube != nil && status.YouTube.Live {
		status.Platforms = addPlatform(status.Platforms, platformYouTube)
		status.Live = true
		return
	}
	if status.Twitch != nil && status.Twitch.Live {
		status.Platforms = addPlatform(status.Platforms, "twitch")
		status.Live = true
		return
	}
	if status.Facebook != nil && status.Facebook.Live {
		status.Platforms = addPlatform(status.Platforms, "facebook")
		status.Live = true
	}
}

func readFile(path string) (File, error) {
	var fileData File
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fileData = File{SchemaRef: DefaultSchemaPath, Records: []Record{}}
			return fileData, nil
		}
		return File{}, fmt.Errorf("read streamers file: %w", err)
	}

	if len(data) == 0 {
		fileData = File{SchemaRef: DefaultSchemaPath, Records: []Record{}}
		return fileData, nil
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		return File{}, fmt.Errorf("parse streamers file: %w", err)
	}
	return fileData, nil
}
