package service

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

const defaultMetadataTimeout = 5 * time.Second

// Metadata captures the resolved metadata for a public URL.
type Metadata struct {
	Description string
	Title       string
	Handle      string
	ChannelID   string
}

// MetadataService fetches metadata for user-supplied URLs.
type MetadataService struct {
	Client  *http.Client
	Timeout time.Duration
}

// Fetch returns the metadata extracted from the provided URL.
func (s MetadataService) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	target, err := normaliseMetadataURL(rawURL)
	if err != nil {
		return Metadata{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.requestTimeout())
	defer cancel()

	desc, title, handle, channelID, fetchErr := s.fetchMetadata(ctx, target)
	if fetchErr != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrUpstream, fetchErr)
	}
	return Metadata{
		Description: desc,
		Title:       title,
		Handle:      handle,
		ChannelID:   channelID,
	}, nil
}

func (s MetadataService) requestTimeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return defaultMetadataTimeout
}

func (s MetadataService) httpClient() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: s.requestTimeout()}
}

func (s MetadataService) fetchMetadata(ctx context.Context, target string) (string, string, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", "", "", err
	}

	resp, err := s.httpClient().Do(req)
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

func normaliseMetadataURL(raw string) (string, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return "", fmt.Errorf("%w: url is required", ErrValidation)
	}

	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%w: url must be http or https", ErrValidation)
	}

	return parsed.String(), nil
}

// EncodeMetadataResponse is used by tests to mimic handler JSON responses.
func EncodeMetadataResponse(w io.Writer, data Metadata) error {
	return json.NewEncoder(w).Encode(data)
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
