package streamers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultSchemaRef = "../schema/streamers.schema.json"

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
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	Description string `json:"description,omitempty"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Email       string `json:"email"`
	City        string `json:"city,omitempty"`
	Country     string `json:"country,omitempty"`
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

var fileMu sync.Mutex

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
		fileData.SchemaRef = defaultSchemaRef
	}

	if record.Streamer.ID == "" {
		record.Streamer.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

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

func readFile(path string) (File, error) {
	var fileData File
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fileData = File{SchemaRef: defaultSchemaRef, Records: []Record{}}
			return fileData, nil
		}
		return File{}, fmt.Errorf("read streamers file: %w", err)
	}

	if len(data) == 0 {
		fileData = File{SchemaRef: defaultSchemaRef, Records: []Record{}}
		return fileData, nil
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		return File{}, fmt.Errorf("parse streamers file: %w", err)
	}
	return fileData, nil
}
