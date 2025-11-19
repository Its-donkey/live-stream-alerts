package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	streamersvc "live-stream-alerts/internal/streamers/service"
)

type patchRequest struct {
	Streamer struct {
		ID          string    `json:"id"`
		Alias       *string   `json:"alias"`
		Description *string   `json:"description"`
		Languages   *[]string `json:"languages"`
	} `json:"streamer"`
}

func (h *streamersHTTPHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	updateReq := streamersvc.UpdateRequest{
		ID:          streamerID,
		Alias:       req.Streamer.Alias,
		Description: req.Streamer.Description,
		Languages:   req.Streamer.Languages,
	}
	record, err := h.service.Update(r.Context(), updateReq)
	if err != nil {
		h.respondError(w, err, "failed to update streamer")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(record)
}
