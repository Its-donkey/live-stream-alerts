package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

const defaultStreamersFile = "data/streamers.json"

// CreateOptions configures the streamer creation handler.
type CreateOptions struct {
	FilePath string
	Logger   logging.Logger
}

// NewCreateHandler returns a handler for POST /api/v1/streamers that appends a new record.
func NewCreateHandler(opts CreateOptions) http.Handler {
	path := opts.FilePath
	if path == "" {
		path = defaultStreamersFile
	}
	path = filepath.Clean(path)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()
		var req streamers.Record
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		if err := validateRecord(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// IDs are server-managed. Ignore any client-provided value.
		req.Streamer.ID = ""
		req.CreatedAt = time.Time{}
		req.UpdatedAt = time.Time{}

		record, err := streamers.Append(path, req)
		if err != nil {
			if opts.Logger != nil {
				opts.Logger.Printf("failed to append streamer: %v", err)
			}
			http.Error(w, "failed to persist streamer", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(record)
	})
}

func validateRecord(record *streamers.Record) error {
	record.Streamer.Alias = strings.TrimSpace(record.Streamer.Alias)
	record.Streamer.FirstName = strings.TrimSpace(record.Streamer.FirstName)
	record.Streamer.LastName = strings.TrimSpace(record.Streamer.LastName)
	record.Streamer.Email = strings.TrimSpace(record.Streamer.Email)
	if record.Streamer.Alias == "" {
		return fmt.Errorf("streamer.alias is required")
	}
	if record.Platforms.YouTube != nil {
		record.Platforms.YouTube.Handle = strings.TrimSpace(record.Platforms.YouTube.Handle)
		if record.Platforms.YouTube.Handle == "" {
			return fmt.Errorf("platforms.youtube.handle is required when youtube is provided")
		}
	}
	return nil
}
