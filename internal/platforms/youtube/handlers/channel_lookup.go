package handlers

import (
	"encoding/json"
	"net/http"

	youtubeclient "live-stream-alerts/internal/platforms/youtube/client"
)

// ChannelLookupHandler resolves a YouTube handle to its canonical channel ID.
func ChannelLookupHandler(client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var handle string

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		defer r.Body.Close()
		
		var payload struct {
			Handle string `json:"handle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		handle = payload.Handle

		if handle == "" {
			http.Error(w, "there is no value for handle set", http.StatusBadRequest)
			return
		}

		channelID, err := youtubeclient.ResolveChannelID(r.Context(), handle, client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{
			"handle":    handle,
			"channelId": channelID,
		})
	})
}