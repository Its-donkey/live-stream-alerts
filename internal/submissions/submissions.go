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
	// ErrNotFound is returned when a submission ID cannot be located.
	ErrNotFound = errors.New("submission not found")
)

// Store persists submissions to disk behind a per-path mutex.
type Store struct {
	path        string
	mu          sync.Mutex
	now         func() time.Time
	idGenerator func() string
}

var storeCache sync.Map

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

// StoreOption customises the store behaviour.
type StoreOption func(*Store)

// WithNow allows tests to override the clock used when populating SubmittedAt.
func WithNow(fn func() time.Time) StoreOption {
	return func(s *Store) {
		if fn != nil {
			s.now = fn
		}
	}
}

// WithIDGenerator overrides how submission IDs are generated when Append is called.
func WithIDGenerator(fn func() string) StoreOption {
	return func(s *Store) {
		if fn != nil {
			s.idGenerator = fn
		}
	}
}

// NewStore returns a file-backed submissions store for the provided path.
func NewStore(path string, opts ...StoreOption) *Store {
	if path == "" {
		path = DefaultFilePath
	}
	store := &Store{
		path:        filepath.Clean(path),
		now:         time.Now,
		idGenerator: defaultSubmissionID,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store
}

// Path returns the path backing the store.
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

func (s *Store) readFileLocked() (File, error) {
	return readFile(s.path)
}

func (s *Store) writeFileLocked(file File) error {
	return writeFile(s.path, file)
}

// List returns every submission recorded at the provided path.
func (s *Store) List() ([]Submission, error) {
	if s == nil {
		return nil, errors.New("submissions store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readFileLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Submission, len(file.Submissions))
	copy(out, file.Submissions)
	return out, nil
}

// List returns submissions using a shared store derived from the provided path.
func List(path string) ([]Submission, error) {
	return storeForPath(path).List()
}

// Remove deletes the submission with the specified ID and returns it.
func (s *Store) Remove(id string) (Submission, error) {
	if s == nil {
		return Submission{}, errors.New("submissions store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readFileLocked()
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
	if err := s.writeFileLocked(file); err != nil {
		return Submission{}, err
	}
	return removed, nil
}

// Remove deletes the submission using a shared store derived from the path.
func Remove(path, id string) (Submission, error) {
	return storeForPath(path).Remove(id)
}

// Append stores a new submission entry.
func (s *Store) Append(submission Submission) (Submission, error) {
	if s == nil {
		return Submission{}, errors.New("submissions store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readFileLocked()
	if err != nil {
		return Submission{}, err
	}
	copy := submission
	if copy.ID == "" {
		copy.ID = s.idGenerator()
	}
	if copy.SubmittedAt.IsZero() {
		copy.SubmittedAt = s.now().UTC()
	}
	file.Submissions = append(file.Submissions, copy)
	if err := s.writeFileLocked(file); err != nil {
		return Submission{}, err
	}
	return copy, nil
}

func defaultSubmissionID() string {
	return fmt.Sprintf("sub_%d", time.Now().UnixNano())
}

// Append stores a submission using a shared store derived from the provided path.
func Append(path string, submission Submission) (Submission, error) {
	return storeForPath(path).Append(submission)
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
