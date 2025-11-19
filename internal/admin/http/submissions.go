package adminhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/config"
	adminauth "live-stream-alerts/internal/admin/auth"
	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/onboarding"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
)

type SubmissionsHandlerOptions struct {
	Manager          *adminauth.Manager
	SubmissionsStore *submissions.Store
	StreamersStore   *streamers.Store
	YouTubeClient    *http.Client
	Logger           logging.Logger
	YouTube          config.YouTubeConfig
}

type submissionsHandler struct {
	manager          *adminauth.Manager
	submissionsStore *submissions.Store
	streamersStore   *streamers.Store
	youtubeClient    *http.Client
	youtube          config.YouTubeConfig
	logger           logging.Logger
}

type adminActionRequest struct {
	Action string `json:"action"`
	ID     string `json:"id"`
}

type submissionsResponse struct {
	Submissions []submissions.Submission `json:"submissions"`
}

func NewSubmissionsHandler(opts SubmissionsHandlerOptions) http.Handler {
	submissionsStore := opts.SubmissionsStore
	if submissionsStore == nil {
		submissionsStore = submissions.NewStore(submissions.DefaultFilePath)
	}
	streamersStore := opts.StreamersStore
	if streamersStore == nil {
		streamersStore = streamers.NewStore(streamers.DefaultFilePath)
	}
	client := opts.YouTubeClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return submissionsHandler{
		manager:          opts.Manager,
		submissionsStore: submissionsStore,
		streamersStore:   streamersStore,
		youtubeClient:    client,
		youtube:          opts.YouTube,
		logger:           opts.Logger,
	}
}

func (h submissionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.list(w)
	case http.MethodPost:
		h.update(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h submissionsHandler) authorize(r *http.Request) bool {
	if h.manager == nil {
		return false
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return h.manager.Validate(parts[1])
}

func (h submissionsHandler) list(w http.ResponseWriter) {
	pending, err := h.submissionsStore.List()
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("list submissions: %v", err)
		}
		http.Error(w, "failed to load submissions", http.StatusInternalServerError)
		return
	}
	respondJSON(w, submissionsResponse{Submissions: pending})
}

func (h submissionsHandler) update(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req adminActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "approve" && action != "reject" {
		http.Error(w, "action must be approve or reject", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	removed, err := h.submissionsStore.Remove(id)
	if err != nil {
		if err == submissions.ErrNotFound {
			http.Error(w, "submission not found", http.StatusNotFound)
			return
		}
		if h.logger != nil {
			h.logger.Printf("update submission: %v", err)
		}
		http.Error(w, "failed to update submission", http.StatusInternalServerError)
		return
	}

	if action == "approve" {
		record := streamers.Record{
			Streamer: streamers.Streamer{
				ID:          streamers.GenerateID(),
				Alias:       removed.Alias,
				Description: removed.Description,
				Languages:   removed.Languages,
			},
		}
		persisted, err := h.streamersStore.Append(record)
		if err != nil {
			if h.logger != nil {
				h.logger.Printf("append streamer from submission: %v", err)
			}
			// requeue submission so it isn't lost
			_ = requeueSubmission(h.submissionsStore, removed, h.logger)
			if errors.Is(err, streamers.ErrDuplicateAlias) {
				http.Error(w, "a streamer with that alias already exists", http.StatusConflict)
				return
			}
			http.Error(w, "failed to approve submission", http.StatusInternalServerError)
			return
		}
		if url := strings.TrimSpace(removed.PlatformURL); url != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			onboardOpts := onboarding.Options{
				Client:        h.youtubeClient,
				HubURL:        strings.TrimSpace(h.youtube.HubURL),
				CallbackURL:   strings.TrimSpace(h.youtube.CallbackURL),
				VerifyMode:    strings.TrimSpace(h.youtube.Verify),
				LeaseSeconds:  h.youtube.LeaseSeconds,
				Logger:        h.logger,
				Store:         h.streamersStore,
			}
			if err := onboarding.FromURL(ctx, persisted, url, onboardOpts); err != nil && h.logger != nil {
				h.logger.Printf("failed to process platform url for %s: %v", persisted.Streamer.Alias, err)
			}
		}
		respondJSON(w, map[string]any{
			"status":     "approved",
			"submission": removed,
		})
		return
	}

	respondJSON(w, map[string]any{
		"status":     "rejected",
		"submission": removed,
	})
}

func respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

func requeueSubmission(store *submissions.Store, submission submissions.Submission, logger logging.Logger) error {
	_, err := store.Append(submission)
	if err != nil && logger != nil {
		logger.Printf("failed to requeue submission %s: %v", submission.ID, err)
	}
	return err
}
