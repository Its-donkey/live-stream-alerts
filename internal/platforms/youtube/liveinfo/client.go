package liveinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// Client fetches live metadata by scraping YouTube watch pages (no API key required).
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

// VideoInfo represents the parsed metadata for a video.
type VideoInfo struct {
	ID                   string
	ChannelID            string
	Title                string
	LiveBroadcastContent string
	ActualStartTime      time.Time
}

// IsLive reports whether the video is currently live.
func (v VideoInfo) IsLive() bool {
	if strings.EqualFold(v.LiveBroadcastContent, "live") {
		return true
	}
	return !v.ActualStartTime.IsZero()
}

// Fetch retrieves metadata for each supplied video ID.
func (c *Client) Fetch(ctx context.Context, videoIDs []string) (map[string]VideoInfo, error) {
	ids := sanitizeIDs(videoIDs)
	if len(ids) == 0 {
		return map[string]VideoInfo{}, nil
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "https://www.youtube.com/watch"
	}

	results := make(map[string]VideoInfo, len(ids))
	var firstErr error
	for _, id := range ids {
		info, err := c.fetchSingle(ctx, httpClient, baseURL, id)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("fetch %s: %w", id, err)
			}
			continue
		}
		results[id] = info
	}
	if len(results) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func (c *Client) fetchSingle(ctx context.Context, client *http.Client, baseURL, id string) (VideoInfo, error) {
	watchURL, err := url.Parse(baseURL)
	if err != nil {
		return VideoInfo{}, err
	}
	q := watchURL.Query()
	q.Set("v", id)
	watchURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, watchURL.String(), nil)
	if err != nil {
		return VideoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LiveStreamAlerts/1.0)")
	req.Header.Set("Accept-Language", "en")

	resp, err := client.Do(req)
	if err != nil {
		return VideoInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return VideoInfo{}, fmt.Errorf("unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return VideoInfo{}, err
	}

	playerJSON, err := extractPlayerResponse(string(body))
	if err != nil {
		return VideoInfo{}, err
	}

	playerJSON = sanitizeJSON(playerJSON)

	var payload playerResponse
	if err := json.Unmarshal([]byte(playerJSON), &payload); err != nil {
		return VideoInfo{}, fmt.Errorf("decode player response: %w", err)
	}

	info := VideoInfo{
		ID:        id,
		ChannelID: payload.VideoDetails.ChannelID,
		Title:     payload.VideoDetails.Title,
	}
	if payload.VideoDetails.IsLiveContent || payload.VideoDetails.IsLive {
		info.LiveBroadcastContent = "live"
	}
	info.ActualStartTime = parseRFC3339(payload.Microformat.PlayerMicroformatRenderer.LiveBroadcastDetails.StartTimestamp)
	return info, nil
}

func extractPlayerResponse(body string) (string, error) {
	marker := "ytInitialPlayerResponse"
	idx := strings.Index(body, marker)
	if idx == -1 {
		return "", errors.New("player response not found")
	}
	start := idx + len(marker)
	for start < len(body) && (body[start] == ' ' || body[start] == '\n' || body[start] == '\r' || body[start] == '\t' || body[start] == '=') {
		start++
	}
	for start < len(body) && unicode.IsSpace(rune(body[start])) {
		start++
	}
	if start >= len(body) {
		return "", errors.New("player response malformed")
	}
	for start < len(body) && body[start] != '{' {
		start++
	}
	if start >= len(body) {
		return "", errors.New("player response JSON missing")
	}

	braces := 0
	end := -1
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '{':
			braces++
		case '}':
			braces--
			if braces == 0 {
				end = i + 1
				break
			}
		}
	}
	if end == -1 {
		return "", errors.New("player response incomplete")
	}
	return body[start:end], nil
}

func sanitizeJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	for len(trimmed) > 0 && trimmed[len(trimmed)-1] == ';' {
		trimmed = strings.TrimSpace(trimmed[:len(trimmed)-1])
	}
	return trimmed
}

func sanitizeIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func parseRFC3339(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return ts
}

type playerResponse struct {
	VideoDetails struct {
		VideoID       string `json:"videoId"`
		ChannelID     string `json:"channelId"`
		Title         string `json:"title"`
		IsLive        bool   `json:"isLive"`
		IsLiveContent bool   `json:"isLiveContent"`
	} `json:"videoDetails"`
	Microformat struct {
		PlayerMicroformatRenderer struct {
			LiveBroadcastDetails struct {
				StartTimestamp string `json:"startTimestamp"`
			} `json:"liveBroadcastDetails"`
		} `json:"playerMicroformatRenderer"`
	} `json:"microformat"`
}
