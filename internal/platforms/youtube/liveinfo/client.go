package liveinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client fetches live metadata for YouTube videos via the Data API.
type Client struct {
	APIKey     string
	HTTPClient *http.Client
	BaseURL    string
}

// VideoInfo describes the metadata returned for a video.
type VideoInfo struct {
	ID                   string
	ChannelID            string
	Title                string
	LiveBroadcastContent string
	ActualStartTime      time.Time
	ScheduledStartTime   time.Time
}

// IsLive returns true when the video is currently live (or has just started).
func (v VideoInfo) IsLive() bool {
	if v.LiveBroadcastContent != "" && strings.EqualFold(v.LiveBroadcastContent, "live") {
		return true
	}
	return !v.ActualStartTime.IsZero()
}

// Fetch returns live metadata for the provided video IDs.
func (c *Client) Fetch(ctx context.Context, videoIDs []string) (map[string]VideoInfo, error) {
	ids := sanitizeIDs(videoIDs)
	if len(ids) == 0 {
		return map[string]VideoInfo{}, nil
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("youtube data api key is required")
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "https://www.googleapis.com/youtube/v3/videos"
	}

	result := make(map[string]VideoInfo, len(ids))
	for start := 0; start < len(ids); start += 50 {
		end := start + 50
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		batchRes, err := c.fetchBatch(ctx, httpClient, baseURL, batch)
		if err != nil {
			return nil, err
		}
		for k, v := range batchRes {
			result[k] = v
		}
	}
	return result, nil
}

func (c *Client) fetchBatch(ctx context.Context, client *http.Client, baseURL string, ids []string) (map[string]VideoInfo, error) {
	params := url.Values{}
	params.Set("part", "snippet,liveStreamingDetails")
	params.Set("id", strings.Join(ids, ","))
	params.Set("key", strings.TrimSpace(c.APIKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube api returned %s", resp.Status)
	}

	var payload apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	items := make(map[string]VideoInfo, len(payload.Items))
	for _, item := range payload.Items {
		info := VideoInfo{
			ID:                   item.ID,
			ChannelID:            item.Snippet.ChannelID,
			Title:                item.Snippet.Title,
			LiveBroadcastContent: item.Snippet.LiveBroadcastContent,
			ActualStartTime:      parseRFC3339(item.LiveStreamingDetails.ActualStartTime),
			ScheduledStartTime:   parseRFC3339(item.LiveStreamingDetails.ScheduledStartTime),
		}
		items[info.ID] = info
	}
	return items, nil
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
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

type apiResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			ChannelID            string `json:"channelId"`
			Title                string `json:"title"`
			LiveBroadcastContent string `json:"liveBroadcastContent"`
		} `json:"snippet"`
		LiveStreamingDetails struct {
			ActualStartTime    string `json:"actualStartTime"`
			ScheduledStartTime string `json:"scheduledStartTime"`
		} `json:"liveStreamingDetails"`
	} `json:"items"`
}
