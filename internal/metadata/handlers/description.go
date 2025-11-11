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

		desc, title, err := fetchDescription(ctx, client, parsed.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(DescriptionResponse{Description: desc, Title: title})
	})
}

func fetchDescription(ctx context.Context, client *http.Client, target string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 2<<20) // 2 MB
	doc, err := goquery.NewDocumentFromReader(limited)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse page")
	}

	if desc, ok := doc.Find(`meta[name="description"]`).Attr("content"); ok {
		if trimmed := strings.TrimSpace(desc); trimmed != "" {
			return trimmed, strings.TrimSpace(doc.Find("title").Text()), nil
		}
	}
	if desc, ok := doc.Find(`meta[property="og:description"]`).Attr("content"); ok {
		if trimmed := strings.TrimSpace(desc); trimmed != "" {
			return trimmed, strings.TrimSpace(doc.Find("title").Text()), nil
		}
	}

	title := strings.TrimSpace(doc.Find("title").Text())
	if title == "" {
		return "", "", fmt.Errorf("description not found")
	}
	return title, title, nil
}
