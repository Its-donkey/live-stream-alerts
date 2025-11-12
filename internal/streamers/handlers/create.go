package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"live-stream-alerts/internal/logging"
	youtubeclient "live-stream-alerts/internal/platforms/youtube/client"
	"live-stream-alerts/internal/streamers"
)

const defaultStreamersFile = "data/streamers.json"

// CreateOptions configures the streamer handler.
type CreateOptions struct {
	FilePath      string
	Logger        logging.Logger
	YouTubeClient *http.Client
	YouTubeHubURL string
}

// NewCreateHandler returns a handler for GET/POST /api/v1/streamers.
func NewCreateHandler(opts CreateOptions) http.Handler {
	path := opts.FilePath
	if path == "" {
		path = defaultStreamersFile
	}
	path = filepath.Clean(path)

	youtubeClient := opts.YouTubeClient
	if youtubeClient == nil {
		youtubeClient = &http.Client{Timeout: 10 * time.Second}
	}
	youtubeHubURL := strings.TrimSpace(opts.YouTubeHubURL)
	if youtubeHubURL == "" {
		youtubeHubURL = youtubeclient.DefaultHubURL
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listStreamers(w, path, opts.Logger)
			return
		case http.MethodPost:
			createStreamer(w, r, path, opts.Logger, youtubeClient, youtubeHubURL)
			return
		default:
			w.Header().Set("Allow", fmt.Sprintf("%s, %s", http.MethodGet, http.MethodPost))
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})
}

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

func createStreamer(w http.ResponseWriter, r *http.Request, path string, logger logging.Logger, youtubeClient *http.Client, youtubeHubURL string) {
	defer r.Body.Close()
	var req streamers.Record
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := validateRecord(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.CreatedAt = time.Time{}
	req.UpdatedAt = time.Time{}

	record, err := streamers.Append(path, req)
	if err != nil {
		if errors.Is(err, streamers.ErrDuplicateStreamerID) {
			http.Error(w, "a streamer with that alias already exists", http.StatusConflict)
			return
		}
		if logger != nil {
			logger.Printf("failed to append streamer: %v", err)
		}
		http.Error(w, "failed to persist streamer", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(record)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := subscribeToYouTubeAlerts(ctx, youtubeClient, youtubeHubURL, record); err != nil && logger != nil {
		logger.Printf("failed to subscribe YouTube alerts for %s: %v", record.Streamer.Alias, err)
	}
}

func validateRecord(record *streamers.Record) error {
	record.Streamer.Alias = strings.TrimSpace(record.Streamer.Alias)
	record.Streamer.FirstName = strings.TrimSpace(record.Streamer.FirstName)
	record.Streamer.LastName = strings.TrimSpace(record.Streamer.LastName)
	record.Streamer.Email = strings.TrimSpace(record.Streamer.Email)
	if record.Streamer.Alias == "" {
		return fmt.Errorf("streamer.alias is required")
	}
	if sanitised := sanitiseAliasForID(record.Streamer.Alias); sanitised == "" {
		return fmt.Errorf("streamer.alias must contain at least one letter or digit")
	} else {
		record.Streamer.ID = sanitised
	}

	languages, err := sanitiseLanguages(record.Streamer.Languages)
	if err != nil {
		return err
	}
	record.Streamer.Languages = languages

	if record.Platforms.YouTube != nil {
		record.Platforms.YouTube.Handle = strings.TrimSpace(record.Platforms.YouTube.Handle)
		if record.Platforms.YouTube.Handle == "" {
			return fmt.Errorf("platforms.youtube.handle is required when youtube is provided")
		}
	}
	return nil
}

func sanitiseAliasForID(alias string) string {
	var builder strings.Builder
	for _, r := range alias {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func sanitiseLanguages(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(values))
	clean := make([]string, 0, len(values))

	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("streamer.languages contains an empty entry")
		}
		if _, ok := allowedLanguagesSet[trimmed]; !ok {
			return nil, fmt.Errorf("streamer.languages contains unsupported value %q", trimmed)
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		clean = append(clean, trimmed)
	}
	return clean, nil
}

func subscribeToYouTubeAlerts(ctx context.Context, client *http.Client, hubURL string, record streamers.Record) error {
	if record.Platforms.YouTube == nil {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	yt := record.Platforms.YouTube
	channelID := strings.TrimSpace(yt.ChannelID)
	handle := strings.TrimSpace(yt.Handle)

	if channelID == "" && handle != "" {
		resolvedID, err := youtubeclient.ResolveChannelID(ctx, handle, client)
		if err != nil {
			return fmt.Errorf("resolve channel ID for handle %s: %w", handle, err)
		}
		channelID = resolvedID
	}
	if channelID == "" {
		return fmt.Errorf("youtube channel ID missing; cannot subscribe")
	}

	topic := fmt.Sprintf("https://www.youtube.com/xml/feeds/videos.xml?channel_id=%s", channelID)
	subscribeReq := youtubeclient.YouTubeRequest{
		Topic:     topic,
		Secret:    strings.TrimSpace(yt.HubSecret),
		Verify:    "async",
		ChannelID: channelID,
	}
	youtubeclient.NormaliseSubscribeRequest(&subscribeReq)

	hubURL = strings.TrimSpace(hubURL)
	if hubURL == "" {
		hubURL = youtubeclient.DefaultHubURL
	}

	_, _, err := youtubeclient.SubscribeYouTube(ctx, client, hubURL, subscribeReq)
	if err != nil {
		return fmt.Errorf("subscribe youtube alerts: %w", err)
	}
	return nil
}

var allowedLanguagesSet = func() map[string]struct{} {
	values := []string{
		"English",
		"Afrikaans",
		"Albanian",
		"Amharic",
		"Armenian",
		"Azerbaijani",
		"Basque",
		"Belarusian",
		"Bosnian",
		"Bulgarian",
		"Catalan",
		"Cebuano",
		"Croatian",
		"Czech",
		"Danish",
		"Dutch",
		"Estonian",
		"Filipino",
		"Finnish",
		"Galician",
		"Georgian",
		"German",
		"Greek",
		"Gujarati",
		"Haitian Creole",
		"Hebrew",
		"Hmong",
		"Hungarian",
		"Icelandic",
		"Igbo",
		"Italian",
		"Japanese",
		"Javanese",
		"Kannada",
		"Kazakh",
		"Khmer",
		"Kinyarwanda",
		"Korean",
		"Kurdish",
		"Lao",
		"Latvian",
		"Lithuanian",
		"Luxembourgish",
		"Macedonian",
		"Malay",
		"Malayalam",
		"Maltese",
		"Marathi",
		"Mongolian",
		"Nepali",
		"Norwegian",
		"Pashto",
		"Persian",
		"Polish",
		"Punjabi",
		"Romanian",
		"Serbian",
		"Sinhala",
		"Slovak",
		"Slovenian",
		"Somali",
		"Swahili",
		"Swedish",
		"Tamil",
		"Telugu",
		"Thai",
		"Turkish",
		"Ukrainian",
		"Urdu",
		"Uzbek",
		"Vietnamese",
		"Welsh",
		"Xhosa",
		"Yoruba",
		"Zulu",
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}()
