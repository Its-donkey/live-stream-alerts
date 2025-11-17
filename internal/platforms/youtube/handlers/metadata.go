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

// MetadataRequest describes the payload for fetching metadata.
type MetadataRequest struct {
	URL string `json:"url"`
}

// MetadataResponse represents the metadata information returned to the client.
type MetadataResponse struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	Handle      string `json:"handle"`
	ChannelID   string `json:"channelId"`
}

// MetadataHandlerOptions configures the metadata handler.
type MetadataHandlerOptions struct {
	Client *http.Client
}

// NewMetadataHandler returns an http.Handler that fetches metadata for a given URL.
func NewMetadataHandler(opts MetadataHandlerOptions) http.Handler {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPostRequest(r) {
			writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		defer r.Body.Close()

		req, validation := decodeMetadataRequest(r)
		if !validation.IsValid {
			http.Error(w, validation.Error, http.StatusBadRequest)
			return
		}

		targetURL, targetValidation := normaliseMetadataURL(req.URL)
		if !targetValidation.IsValid {
			http.Error(w, targetValidation.Error, http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		desc, title, handle, channelID, err := fetchMetadata(ctx, client, targetURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		writeMetadataResponse(w, MetadataResponse{
			Description: desc,
			Title:       title,
			Handle:      handle,
			ChannelID:   channelID,
		})
	})
}

func decodeMetadataRequest(r *http.Request) (MetadataRequest, ValidationResult) {
	var req MetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, ValidationResult{IsValid: false, Error: "invalid JSON body"}
	}
	return req, ValidationResult{IsValid: true}
}

func normaliseMetadataURL(raw string) (string, ValidationResult) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", ValidationResult{IsValid: false, Error: "url is required"}
	}

	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ValidationResult{IsValid: false, Error: "url must be http or https"}
	}

	return parsed.String(), ValidationResult{IsValid: true}
}

func writeMetadataResponse(w http.ResponseWriter, resp MetadataResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func fetchMetadata(ctx context.Context, client *http.Client, target string) (string, string, string, string, error) {
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
