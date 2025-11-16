package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/platforms/youtube/onboarding"
	"live-stream-alerts/internal/streamers"
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

func createStreamer(w http.ResponseWriter, r *http.Request, path string, logger logging.Logger, youtubeClient *http.Client, youtubeHubURL string) {
	defer r.Body.Close()
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	record, err := buildRecord(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	record, err = streamers.Append(path, record)
	if err != nil {
		if errors.Is(err, streamers.ErrDuplicateStreamerID) || errors.Is(err, streamers.ErrDuplicateAlias) {
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

	channelURL := strings.TrimSpace(req.Platforms.URL)
	if channelURL == "" {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	onboardOpts := onboarding.Options{
		Client:        youtubeClient,
		HubURL:        youtubeHubURL,
		Logger:        logger,
		StreamersPath: path,
	}
	if err := onboarding.FromURL(ctx, record, channelURL, onboardOpts); err != nil && logger != nil {
		logger.Printf("failed to process YouTube URL for %s: %v", record.Streamer.Alias, err)
	}
}

func buildRecord(req createRequest) (streamers.Record, error) {
	alias := strings.TrimSpace(req.Streamer.Alias)
	if alias == "" {
		return streamers.Record{}, fmt.Errorf("streamer.alias is required")
	}

	if streamers.NormaliseAlias(alias) == "" {
		return streamers.Record{}, fmt.Errorf("streamer.alias must contain at least one letter or digit")
	}

	langs, err := sanitiseLanguages(req.Streamer.Languages)
	if err != nil {
		return streamers.Record{}, err
	}

	record := streamers.Record{
		Streamer: streamers.Streamer{
			ID:          streamers.GenerateID(),
			Alias:       alias,
			Description: strings.TrimSpace(req.Streamer.Description),
			Languages:   langs,
		},
		Platforms: streamers.Platforms{},
	}

	return record, nil
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
