package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

type patchRequest struct {
	Streamer struct {
		ID          string    `json:"id"`
		Alias       *string   `json:"alias"`
		Description *string   `json:"description"`
		Languages   *[]string `json:"languages"`
	} `json:"streamer"`
}

func updateStreamer(w http.ResponseWriter, r *http.Request, store *streamers.Store, logger logging.Logger) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if store == nil {
		http.Error(w, "streamers store not configured", http.StatusInternalServerError)
		return
	}

	defer r.Body.Close()
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	streamerID := strings.TrimSpace(req.Streamer.ID)
	if streamerID == "" {
		http.Error(w, "streamer.id is required", http.StatusBadRequest)
		return
	}

	update := streamers.UpdateFields{
		StreamerID: streamerID,
	}

	var hasUpdate bool
	if req.Streamer.Alias != nil {
		alias := strings.TrimSpace(*req.Streamer.Alias)
		if alias == "" {
			http.Error(w, "streamer.alias cannot be blank", http.StatusBadRequest)
			return
		}
		update.Alias = &alias
		hasUpdate = true
	}
	if req.Streamer.Description != nil {
		desc := strings.TrimSpace(*req.Streamer.Description)
		update.Description = &desc
		hasUpdate = true
	}
	if req.Streamer.Languages != nil {
		langs, err := sanitiseLanguages(*req.Streamer.Languages)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		update.Languages = new([]string)
		*update.Languages = langs
		hasUpdate = true
	}

	if !hasUpdate {
		http.Error(w, "at least one streamer field must be provided", http.StatusBadRequest)
		return
	}

	record, err := store.Update(update)
	if err != nil {
		switch {
		case errors.Is(err, streamers.ErrStreamerNotFound):
			http.Error(w, "streamer not found", http.StatusNotFound)
			return
		default:
			if logger != nil {
				logger.Printf("failed to update streamer %s: %v", streamerID, err)
			}
			http.Error(w, "failed to update streamer", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(record)
}
