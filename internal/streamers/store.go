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
	// ErrDuplicateStreamerID indicates an ID conflict during persistence.
	ErrDuplicateStreamerID = errors.New("streamer id already exists")
	// ErrStreamerNotFound signals that no streamer matched the provided identifier.
	ErrStreamerNotFound = errors.New("streamer not found")
	// ErrDuplicateAlias indicates the alias collides with an existing record.
	ErrDuplicateAlias = errors.New("streamer alias already exists")
)

// Store persists streamer records to a JSON file with per-path locking.
type Store struct {
	path string
	mu   sync.Mutex
}

var storeCache sync.Map

// NewStore returns a file-backed store for the provided path.
func NewStore(path string) *Store {
	if path == "" {
		path = DefaultFilePath
	}
	return &Store{path: filepath.Clean(path)}
}

// Path returns the file path backing the store.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func storeForPath(path string) *Store {
	if path == "" {
		path = DefaultFilePath
	}
	cleaned := filepath.Clean(path)
	if existing, ok := storeCache.Load(cleaned); ok {
		return existing.(*Store)
	}
	store := &Store{path: cleaned}
	actual, _ := storeCache.LoadOrStore(cleaned, store)
	return actual.(*Store)
}

func (s *Store) ensureDir() error {
	if s == nil {
		return errors.New("streamers store is nil")
	}
	return os.MkdirAll(filepath.Dir(s.path), 0o755)
}

func (s *Store) readFileLocked() (File, error) {
	var fileData File
	data, err := os.ReadFile(s.path)
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

func (s *Store) writeFileLocked(file File) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create streamers dir: %w", err)
	}
	encoded, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode streamers file: %w", err)
	}
	if err := os.WriteFile(s.path, encoded, 0o644); err != nil {
		return fmt.Errorf("write streamers file: %w", err)
	}
	return nil
}

func (s *Store) updateFileLocked(updateFn func(*File) error) error {
	fileData, err := s.readFileLocked()
	if err != nil {
		return err
	}
	if err := updateFn(&fileData); err != nil {
		return err
	}
	return s.writeFileLocked(fileData)
}

// UpdateFields describes the mutable streamer fields.
type UpdateFields struct {
	StreamerID  string
	Alias       *string
	Description *string
	Languages   *[]string
}

// Append adds a new streamer record to disk and returns a copy with timestamps populated.
func (s *Store) Append(record Record) (Record, error) {
	if s == nil {
		return Record{}, errors.New("streamers store is nil")
	}
	if err := s.ensureDir(); err != nil {
		return Record{}, fmt.Errorf("create streamers dir: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fileData, err := s.readFileLocked()
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
	if err := s.writeFileLocked(fileData); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Append adds a new streamer record using a shared store derived from the path.
func Append(path string, record Record) (Record, error) {
	return storeForPath(path).Append(record)
}

// List loads all streamer records from disk.
func (s *Store) List() ([]Record, error) {
	if s == nil {
		return nil, errors.New("streamers store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	fileData, err := s.readFileLocked()
	if err != nil {
		return nil, err
	}
	records := make([]Record, len(fileData.Records))
	copy(records, fileData.Records)
	return records, nil
}

// List loads all streamer records for the provided path using a shared store instance.
func List(path string) ([]Record, error) {
	return storeForPath(path).List()
}

// YouTubeLiveStatus describes the live state to persist for a YouTube channel.
type YouTubeLiveStatus struct {
	Live      bool
	VideoID   string
	StartedAt time.Time
}

// UpdateYouTubeLiveStatus updates the stored status for the streamer owning the channel ID.
func (s *Store) UpdateYouTubeLiveStatus(channelID string, liveStatus YouTubeLiveStatus) (Record, error) {
	if s == nil {
		return Record{}, errors.New("streamers store is nil")
	}
	ch := strings.TrimSpace(channelID)
	if ch == "" {
		return Record{}, errors.New("channel id is required")
	}
	var updated Record
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.updateFileLocked(func(file *File) error {
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

// UpdateYouTubeLiveStatus updates the stored status using a shared store instance derived from the provided path.
func UpdateYouTubeLiveStatus(path, channelID string, liveStatus YouTubeLiveStatus) (Record, error) {
	return storeForPath(path).UpdateYouTubeLiveStatus(channelID, liveStatus)
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
func (s *Store) Update(fields UpdateFields) (Record, error) {
	if s == nil {
		return Record{}, errors.New("streamers store is nil")
	}
	id := strings.TrimSpace(fields.StreamerID)
	if id == "" {
		return Record{}, errors.New("streamer id is required")
	}
	if fields.Alias == nil && fields.Description == nil && fields.Languages == nil {
		return Record{}, errors.New("no fields provided to update")
	}

	var updated Record
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.updateFileLocked(func(file *File) error {
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

// Update applies modifications using a shared store derived from the provided path.
func Update(path string, fields UpdateFields) (Record, error) {
	return storeForPath(path).Update(fields)
}

// UpdateFile reads the streamers file, applies the provided mutation, and writes it back to disk atomically.
func (s *Store) UpdateFile(updateFn func(*File) error) error {
	if s == nil {
		return errors.New("streamers store is nil")
	}
	if updateFn == nil {
		return errors.New("updateFn is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateFileLocked(updateFn)
}

// UpdateFile reads and updates the file for the provided path using a shared store instance.
func UpdateFile(path string, updateFn func(*File) error) error {
	return storeForPath(path).UpdateFile(updateFn)
}

// Delete removes a streamer by ID.
func (s *Store) Delete(streamerID string) error {
	if s == nil {
		return errors.New("streamers store is nil")
	}
	streamerID = strings.TrimSpace(streamerID)
	if streamerID == "" {
		return errors.New("streamer id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateFileLocked(func(file *File) error {
		for i := range file.Records {
			if strings.EqualFold(file.Records[i].Streamer.ID, streamerID) {
				file.Records = append(file.Records[:i], file.Records[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrStreamerNotFound, streamerID)
	})
}

// Delete removes a streamer by ID for the provided path using a shared store instance.
func Delete(path, streamerID string) error {
	return storeForPath(path).Delete(streamerID)
}

// Get returns a single streamer record by ID.
func (s *Store) Get(streamerID string) (Record, error) {
	if s == nil {
		return Record{}, errors.New("streamers store is nil")
	}
	streamerID = strings.TrimSpace(streamerID)
	if streamerID == "" {
		return Record{}, errors.New("streamer id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fileData, err := s.readFileLocked()
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

// Get returns a record using a shared store derived from the provided path.
func Get(path, streamerID string) (Record, error) {
	return storeForPath(path).Get(streamerID)
}

// SetYouTubeLive marks the streamer associated with the provided channel ID as live.
func (s *Store) SetYouTubeLive(channelID, videoID string, startedAt time.Time) (Record, error) {
	return s.updateYouTubeStatus(channelID, func(status *Status) {
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
func (s *Store) ClearYouTubeLive(channelID string) (Record, error) {
	return s.updateYouTubeStatus(channelID, func(status *Status) {
		if status.YouTube == nil {
			status.YouTube = &YouTubeStatus{}
		}
		status.YouTube.Live = false
		status.YouTube.VideoID = ""
		status.YouTube.StartedAt = time.Time{}
		status.Platforms = removePlatform(status.Platforms, platformYouTube)
	})
}

// SetYouTubeLive marks the streamer as live using a shared store derived from path.
func SetYouTubeLive(path, channelID, videoID string, startedAt time.Time) (Record, error) {
	return storeForPath(path).SetYouTubeLive(channelID, videoID, startedAt)
}

// ClearYouTubeLive marks the streamer as offline using a shared store.
func ClearYouTubeLive(path, channelID string) (Record, error) {
	return storeForPath(path).ClearYouTubeLive(channelID)
}

const platformYouTube = "youtube"

func (s *Store) updateYouTubeStatus(channelID string, updateFn func(*Status)) (Record, error) {
	if s == nil {
		return Record{}, errors.New("streamers store is nil")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return Record{}, errors.New("youtube channel id is required")
	}
	var updated Record
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.updateFileLocked(func(file *File) error {
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
