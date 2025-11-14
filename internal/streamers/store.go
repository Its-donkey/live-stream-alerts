package streamers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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

// YouTubePlatform stores YouTube-specific metadata.
type YouTubePlatform struct {
	Handle             string `json:"handle"`
	ChannelID          string `json:"channelId,omitempty"`
	HubSecret          string `json:"hubSecret,omitempty"`
	HubLeaseRenewalDue string `json:"hubLeaseRenewalDue,omitempty"`
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
	fileMu                       sync.Mutex
	ErrDuplicateStreamerID       = errors.New("streamer id already exists")
	ErrStreamerNotFound          = errors.New("streamer not found")
	ErrStreamerTimestampMismatch = errors.New("streamer createdAt does not match")
)

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
		record.Streamer.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	for _, existing := range fileData.Records {
		if existing.Streamer.ID == record.Streamer.ID {
			return Record{}, fmt.Errorf("%w: %s", ErrDuplicateStreamerID, record.Streamer.ID)
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

// Delete removes a streamer by ID, ensuring the createdAt timestamp matches.
func Delete(path, streamerID, createdAt string) error {
	if path == "" {
		return errors.New("streamers file path is required")
	}
	streamerID = strings.TrimSpace(streamerID)
	if streamerID == "" {
		return errors.New("streamer id is required")
	}
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return errors.New("createdAt is required")
	}
	parseCreatedAt := func(value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, errors.New("createdAt is required")
		}
		if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return ts.UTC(), nil
		}
		ts, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, err
		}
		return ts.UTC(), nil
	}
	expectedTime, err := parseCreatedAt(createdAt)
	if err != nil {
		return fmt.Errorf("invalid createdAt: %w", err)
	}

	return UpdateFile(path, func(file *File) error {
		for i := range file.Records {
			if strings.EqualFold(file.Records[i].Streamer.ID, streamerID) {
				if !file.Records[i].CreatedAt.Equal(expectedTime) {
					return fmt.Errorf("%w: %s", ErrStreamerTimestampMismatch, streamerID)
				}
				file.Records = append(file.Records[:i], file.Records[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrStreamerNotFound, streamerID)
	})
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
