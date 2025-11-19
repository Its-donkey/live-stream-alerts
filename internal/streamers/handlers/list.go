package handlers

import (
	"encoding/json"
	"net/http"

	"live-stream-alerts/internal/streamers"
)

func (h *streamersHTTPHandler) handleList(w http.ResponseWriter, r *http.Request) {
	records, err := h.service.List(r.Context())
	if err != nil {
		h.respondError(w, err, "failed to read streamer data")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	response := struct {
		Streamers []streamers.Record `json:"streamers"`
	}{
		Streamers: records,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		if h.logger != nil {
			h.logger.Printf("failed to encode streamers response: %v", err)
		}
	}
}
