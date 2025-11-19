package submissions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultFilePath is where pending submissions are stored.
	DefaultFilePath = "data/submissions.json"
)

var (
	fileMu sync.Mutex
	// ErrNotFound is returned when a submission ID cannot be located.
	ErrNotFound = errors.New("submission not found")
)

// File represents the on-disk submissions format.
type File struct {
	SchemaRef   string       `json:"$schema,omitempty"`
	Submissions []Submission `json:"submissions"`
}

// Submission captures the data submitted by a user awaiting admin review.
type Submission struct {
	ID          string    `json:"id"`
	Alias       string    `json:"alias"`
	Description string    `json:"description,omitempty"`
	Languages   []string  `json:"languages,omitempty"`
	PlatformURL string    `json:"platformUrl,omitempty"`
	SubmittedAt time.Time `json:"submittedAt"`
	SubmittedBy string    `json:"submittedBy,omitempty"`
}

// List returns every submission recorded at the provided path.
func List(path string) ([]Submission, error) {
	file, err := readFile(path)
	if err != nil {
		return nil, err
	}
	out := make([]Submission, len(file.Submissions))
	copy(out, file.Submissions)
	return out, nil
}

// Remove deletes the submission with the specified ID and returns it.
func Remove(path, id string) (Submission, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	file, err := readFile(path)
	if err != nil {
		return Submission{}, err
	}
	idx := -1
	for i, sub := range file.Submissions {
		if sub.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return Submission{}, ErrNotFound
	}

	removed := file.Submissions[idx]
	file.Submissions = append(file.Submissions[:idx], file.Submissions[idx+1:]...)
	if err := writeFile(path, file); err != nil {
		return Submission{}, err
	}
	return removed, nil
}

// Append stores a new submission entry.
func Append(path string, submission Submission) (Submission, error) {
	fileMu.Lock()
	defer fileMu.Unlock()

	file, err := readFile(path)
	if err != nil {
		return Submission{}, err
	}
	copy := submission
	if copy.ID == "" {
		copy.ID = fmt.Sprintf("sub_%d", time.Now().UnixNano())
	}
	if copy.SubmittedAt.IsZero() {
		copy.SubmittedAt = time.Now().UTC()
	}
	file.Submissions = append(file.Submissions, copy)
	if err := writeFile(path, file); err != nil {
		return Submission{}, err
	}
	return copy, nil
}

func readFile(path string) (File, error) {
	if path == "" {
		path = DefaultFilePath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{Submissions: []Submission{}}, nil
		}
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("decode submissions file: %w", err)
	}
	if file.Submissions == nil {
		file.Submissions = []Submission{}
	}
	return file, nil
}

func writeFile(path string, file File) error {
	if path == "" {
		path = DefaultFilePath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create submissions dir: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode submissions file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write submissions file: %w", err)
	}
	return nil
}
