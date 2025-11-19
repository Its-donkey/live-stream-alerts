package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	streamersvc "live-stream-alerts/internal/streamers/service"
)

type deleteRequest struct {
	Streamer struct {
		ID string `json:"id"`
	} `json:"streamer"`
}

func (h *streamersHTTPHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var body deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(body.Streamer.ID)
	if id == "" {
		http.Error(w, "streamer.id is required in body", http.StatusBadRequest)
		return
	}
	if err := h.service.Delete(r.Context(), streamersvc.DeleteRequest{ID: id}); err != nil {
		h.respondError(w, err, "failed to delete streamer")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
		"id":     id,
	})
}
