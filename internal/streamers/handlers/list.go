package handlers

import (
	"encoding/json"
	"net/http"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
)

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
