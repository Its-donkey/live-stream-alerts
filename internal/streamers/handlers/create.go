package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

const defaultStreamersFile = "data/streamers.json"

// CreateOptions configures the streamer handler.
type CreateOptions struct {
	FilePath string
	Logger   logging.Logger
}

// NewCreateHandler returns a handler for GET/POST /api/v1/streamers.
func NewCreateHandler(opts CreateOptions) http.Handler {
	path := opts.FilePath
	if path == "" {
		path = defaultStreamersFile
	}
	path = filepath.Clean(path)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listStreamers(w, path, opts.Logger)
			return
		case http.MethodPost:
			createStreamer(w, r, path, opts.Logger)
			return
		default:
			w.Header().Set("Allow", fmt.Sprintf("%s, %s", http.MethodGet, http.MethodPost))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})
}

func listStreamers(w http.ResponseWriter, path string, logger logging.Logger) {
	records, err := streamers.List(path)
	if err != nil {
		if logger != nil {
			logger.Printf("failed to list streamers: %v", err)
		}
		http.Error(w, "failed to read streamer data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	response := struct {
		Streamers []streamers.Record `json:"streamers"`
	}{
		Streamers: records,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil && logger != nil {
		logger.Printf("failed to encode streamers response: %v", err)
	}
}

func createStreamer(w http.ResponseWriter, r *http.Request, path string, logger logging.Logger) {
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

	req.CreatedAt = time.Time{}
	req.UpdatedAt = time.Time{}

	record, err := streamers.Append(path, req)
	if err != nil {
		if logger != nil {
			logger.Printf("failed to append streamer: %v", err)
		}
		http.Error(w, "failed to persist streamer", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(record)
}

func validateRecord(record *streamers.Record) error {
	record.Streamer.Alias = strings.TrimSpace(record.Streamer.Alias)
	record.Streamer.FirstName = strings.TrimSpace(record.Streamer.FirstName)
	record.Streamer.LastName = strings.TrimSpace(record.Streamer.LastName)
	record.Streamer.Email = strings.TrimSpace(record.Streamer.Email)
	if record.Streamer.Alias == "" {
		return fmt.Errorf("streamer.alias is required")
	}
	if sanitized := sanitizeAliasForID(record.Streamer.Alias); sanitized == "" {
		return fmt.Errorf("streamer.alias must contain at least one letter or digit")
	} else {
		record.Streamer.ID = sanitized
	}

	if record.Platforms.YouTube != nil {
		record.Platforms.YouTube.Handle = strings.TrimSpace(record.Platforms.YouTube.Handle)
		if record.Platforms.YouTube.Handle == "" {
			return fmt.Errorf("platforms.youtube.handle is required when youtube is provided")
		}
	}
	return nil
}

func sanitizeAliasForID(alias string) string {
	var builder strings.Builder
	for _, r := range alias {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
