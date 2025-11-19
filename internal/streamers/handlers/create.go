package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"live-stream-alerts/internal/logging"
	"live-stream-alerts/internal/streamers"
	"live-stream-alerts/internal/submissions"
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

func createStreamer(w http.ResponseWriter, r *http.Request, store *streamers.Store, submissionsStore *submissions.Store, logger logging.Logger) {
	if store == nil || submissionsStore == nil {
		http.Error(w, "storage not configured", http.StatusInternalServerError)
		return
	}
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

	if err := ensureUniqueAlias(store, submissionsStore, record.Streamer.Alias); err != nil {
		if errors.Is(err, errDuplicateAlias) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if logger != nil {
			logger.Printf("alias check failed: %v", err)
		}
		http.Error(w, "failed to queue submission", http.StatusInternalServerError)
		return
	}

	submission := submissions.Submission{
		Alias:       record.Streamer.Alias,
		Description: record.Streamer.Description,
		Languages:   record.Streamer.Languages,
		PlatformURL: strings.TrimSpace(req.Platforms.URL),
	}
	if _, err := submissionsStore.Append(submission); err != nil {
		if logger != nil {
			logger.Printf("failed to queue submission: %v", err)
		}
		http.Error(w, "failed to queue submission", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "pending",
		"message": "Submission received and pending approval.",
	})
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

var errDuplicateAlias = fmt.Errorf("a streamer with that alias already exists")

func ensureUniqueAlias(streamerStore *streamers.Store, submissionsStore *submissions.Store, alias string) error {
	key := streamers.NormaliseAlias(alias)
	if key == "" {
		return nil
	}

	records, err := streamerStore.List()
	if err != nil {
		return err
	}
	for _, rec := range records {
		if key == streamers.NormaliseAlias(rec.Streamer.Alias) {
			return errDuplicateAlias
		}
	}

	pending, err := submissionsStore.List()
	if err != nil {
		return err
	}
	for _, sub := range pending {
		if key == streamers.NormaliseAlias(sub.Alias) {
			return errDuplicateAlias
		}
	}
	return nil
}
