package handlers

import (
	"encoding/json"
	"net/http"

	streamersvc "live-stream-alerts/internal/streamers/service"
)

type createRequest struct {
	Streamer struct {
		Alias       string   `json:"alias"`
		Description string   `json:"description"`
		Languages   []string `json:"languages"`
	} `json:"streamer"`
	Platforms struct {
		URL string `json:"url"`
	} `json:"platforms"`
}

func (h *streamersHTTPHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	createReq := streamersvc.CreateRequest{
		Alias:       req.Streamer.Alias,
		Description: req.Streamer.Description,
		Languages:   req.Streamer.Languages,
		PlatformURL: req.Platforms.URL,
	}
	if _, err := h.service.Create(r.Context(), createReq); err != nil {
		h.respondError(w, err, "failed to queue submission")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "pending",
		"message": "Submission received and pending approval.",
	})
}
