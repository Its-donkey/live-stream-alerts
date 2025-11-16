package liveinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"live-stream-alerts/internal/logging"
)

// Client fetches live metadata by scraping YouTube watch pages (no API key required).
type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	Logger     logging.Logger
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
	c.logf("Fetching live metadata for %d video(s): %s", len(ids), strings.Join(ids, ", "))

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
			c.logf("Fetch for video %s failed: %v", id, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("fetch %s: %w", id, err)
			}
			continue
		}
		c.logf("Fetched metadata for %s: channel=%s title=%q live=%v start=%s", id, info.ChannelID, info.Title, info.IsLive(), info.ActualStartTime)
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

	c.logf("Requesting watch page for %s: %s", id, watchURL.String())
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
		c.logf("Failed to locate player response for %s; first 200 bytes: %q", id, previewBody(body, 200))
		return VideoInfo{}, err
	}

	playerJSON = sanitizeJSON(playerJSON)

	var payload playerResponse
	if err := json.Unmarshal([]byte(playerJSON), &payload); err != nil {
		c.logf("Player response decode failed for %s. Payload prefix: %q", id, previewString(playerJSON, 200))
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

func (c *Client) logf(format string, args ...any) {
	if c == nil || c.Logger == nil {
		return
	}
	c.Logger.Printf(format, args...)
}

var playerResponsePattern = regexp.MustCompile(`(?s)ytInitialPlayerResponse\s*=\s*(\{.+?\});`)

func extractPlayerResponse(body string) (string, error) {
	match := playerResponsePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return "", errors.New("player response not found")
	}
	return strings.TrimSpace(match[1]), nil
}

func sanitizeJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	for strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
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

func previewBody(body []byte, limit int) string {
	if len(body) == 0 {
		return ""
	}
	if len(body) > limit {
		return string(body[:limit]) + "…"
	}
	return string(body)
}

func previewString(raw string, limit int) string {
	if len(raw) > limit {
		return raw[:limit] + "…"
	}
	return raw
}
