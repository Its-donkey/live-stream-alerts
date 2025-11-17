package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

type channelLookupRequest struct {
	Handle string `json:"handle"`
}

// NewChannelLookupHandler resolves a YouTube handle to its canonical channel ID.
func NewChannelLookupHandler(client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPostRequest(r) {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		defer r.Body.Close()

		payload, validation := decodeChannelLookupRequest(r)
		if !validation.IsValid {
			http.Error(w, validation.Error, http.StatusBadRequest)
			return
		}

		channelID, err := subscriptions.ResolveChannelID(r.Context(), payload.Handle, client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		writeChannelLookupResponse(w, payload.Handle, channelID)
	})
}

func isPostRequest(r *http.Request) bool {
	return r.Method == http.MethodPost
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func decodeChannelLookupRequest(r *http.Request) (channelLookupRequest, ValidationResult) {
	var payload channelLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return payload, ValidationResult{IsValid: false, Error: "invalid JSON body"}
	}

	payload.Handle = strings.TrimSpace(payload.Handle)
	if payload.Handle == "" {
		return payload, ValidationResult{IsValid: false, Error: "there is no value for handle set"}
	}

	return payload, ValidationResult{IsValid: true}
}

func writeChannelLookupResponse(w http.ResponseWriter, handle, channelID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"handle":    handle,
		"channelId": channelID,
	})
}
