package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// DescriptionRequest describes the payload for fetching metadata.
type DescriptionRequest struct {
	URL string `json:"url"`
}

// DescriptionResponse represents the metadata information returned to the client.
type DescriptionResponse struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	Handle      string `json:"handle"`
	ChannelID   string `json:"channelId"`
}

// DescriptionHandlerOptions configures the description handler.
type DescriptionHandlerOptions struct {
	Client *http.Client
}

// DescriptionHandler returns an http.Handler that fetches the description of a given URL.
func DescriptionHandler(opts DescriptionHandlerOptions) http.Handler {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()
		var req DescriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		target := strings.TrimSpace(req.URL)
		parsed, err := url.Parse(target)
		if err != nil || parsed.Scheme == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			http.Error(w, "url must be http or https", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		desc, title, handle, channelID, err := fetchDescription(ctx, client, parsed.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(DescriptionResponse{
			Description: desc,
			Title:       title,
			Handle:      handle,
			ChannelID:   channelID,
		})
	})
}

func fetchDescription(ctx context.Context, client *http.Client, target string) (string, string, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", "", err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 2<<20) // 2 MB
	doc, err := goquery.NewDocumentFromReader(limited)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to parse page")
	}

	title := firstNonEmpty(
		doc.Find(`meta[property="og:title"]`).AttrOr("content", ""),
		doc.Find(`meta[name="twitter:title"]`).AttrOr("content", ""),
		doc.Find("title").Text(),
	)
	desc := firstNonEmpty(
		doc.Find(`meta[name="description"]`).AttrOr("content", ""),
		doc.Find(`meta[property="og:description"]`).AttrOr("content", ""),
	)
	if desc == "" {
		desc = title
	}
	if desc == "" && title == "" {
		return "", "", "", "", fmt.Errorf("description not found")
	}

	pageURL := firstNonEmpty(
		doc.Find(`meta[property="og:url"]`).AttrOr("content", ""),
		doc.Find(`link[rel="canonical"]`).AttrOr("href", ""),
		target,
	)
	handle := deriveHandle(pageURL)
	channelID := firstNonEmpty(
		doc.Find(`meta[itemprop="channelId"]`).AttrOr("content", ""),
		parseChannelID(pageURL),
	)
	if handle == "" {
		handle = deriveHandle(target)
	}
	if channelID == "" {
		channelID = parseChannelID(target)
	}
	return desc, title, handle, channelID, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func deriveHandle(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	if strings.HasPrefix(parts[0], "@") {
		return parts[0]
	}
	return ""
}

func parseChannelID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "channel" {
		return parts[1]
	}
	return ""
}
