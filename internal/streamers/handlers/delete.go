// file name — /internal/streamers/handlers/delete.go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/subscriptions"
	"live-stream-alerts/internal/streamers"
)

type deleteRequest struct {
	Streamer struct {
		ID string `json:"id"`
	} `json:"streamer"`
}

func deleteStreamer(w http.ResponseWriter, r *http.Request, store *streamers.Store, logger logging.Logger, youtubeClient *http.Client, youtubeHubURL string) {
	// Enforce method (defensive check; switch already filtered by method)
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if store == nil {
		http.Error(w, "streamers store not configured", http.StatusInternalServerError)
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

	record, err := store.Get(bodyID)
	if err != nil {
		switch {
		case errors.Is(err, streamers.ErrStreamerNotFound):
			http.Error(w, "streamer not found", http.StatusNotFound)
			return
		default:
			if logger != nil {
				logger.Printf("failed to load streamer %s: %v", bodyID, err)
			}
			http.Error(w, "failed to load streamer", http.StatusInternalServerError)
			return
		}
	}

	if record.Platforms.YouTube != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		unsubOpts := subscriptions.Options{
			Client: youtubeClient,
			HubURL: youtubeHubURL,
			Logger: logger,
			Mode:   "unsubscribe",
		}
		if err := subscriptions.ManageSubscription(ctx, record, unsubOpts); err != nil {
			if logger != nil {
				logger.Printf("failed to unsubscribe alerts for %s: %v", record.Streamer.ID, err)
			}
			http.Error(w, "failed to unsubscribe alerts", http.StatusBadGateway)
			return
		}
	}

	// Perform delete via streamers.Delete
	if err := store.Delete(bodyID); err != nil {
		switch {
		case errors.Is(err, streamers.ErrStreamerNotFound):
			http.Error(w, "streamer not found", http.StatusNotFound)
			return
		default:
			if logger != nil {
				logger.Printf("failed to delete streamer %s: %v", bodyID, err)
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
		"id":     bodyID,
	})
}
