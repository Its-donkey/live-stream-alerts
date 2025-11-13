package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	channelIDPattern = regexp.MustCompile(`"channelId":"(UC[\w-]{22})"`)
	handlePrefix     = "@"
)

// ResolveChannelID fetches a YouTube handle page (/@handle/about) and extracts the canonical channel ID (UC...).
// It requires no API key and relies on the HTML payload returned by YouTube.
func ResolveChannelID(ctx context.Context, handle string, client *http.Client) (string, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return "", errors.New("handle is required")
	}
	if !strings.HasPrefix(handle, handlePrefix) {
		handle = handlePrefix + handle
	}

	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	url := fmt.Sprintf("https://www.youtube.com/%s/about", handle)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LiveStreamAlerts/1.0)")
	req.Header.Set("Accept-Language", "en")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch handle page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s when resolving handle", resp.Status)
	}

	// Limit read to 2MB to guard against excessive payload.
	const maxBody = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("read handle page: %w", err)
	}

	matches := channelIDPattern.FindSubmatch(body)
	if len(matches) != 2 {
		return "", errors.New("channel ID not found in handle page")
	}
	return string(matches[1]), nil
}
