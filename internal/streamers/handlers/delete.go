// file name — /internal/streamers/handlers/delete.go
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

type deleteRequest struct {
	Streamer struct {
		ID string `json:"id"`
	} `json:"streamer"`
}

func deleteStreamer(w http.ResponseWriter, r *http.Request, path string, logger logging.Logger) {
	// Enforce method (defensive check; switch already filtered by method)
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL: /api/streamers/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/streamers/")
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		http.Error(w, "streamer id is required in path", http.StatusBadRequest)
		return
	}

	// Parse JSON body
	defer r.Body.Close()
	var body deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate body.id
	bodyID := strings.TrimSpace(body.Streamer.ID)
	if bodyID == "" {
		http.Error(w, "streamer.id is required in body", http.StatusBadRequest)
		return
	}
	if !strings.EqualFold(bodyID, id) {
		http.Error(w, "streamer.id mismatch between path and body", http.StatusBadRequest)
		return
	}

	// Perform delete via streamers.Delete
	if err := streamers.Delete(path, id); err != nil {
		switch {
		case errors.Is(err, streamers.ErrStreamerNotFound):
			http.Error(w, "streamer not found", http.StatusNotFound)
			return
		default:
			if logger != nil {
				logger.Printf("failed to delete streamer %s: %v", id, err)
			}
			http.Error(w, "failed to delete streamer", http.StatusInternalServerError)
			return
		}
	}

	// Success
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
		"id":     id,
	})
}
