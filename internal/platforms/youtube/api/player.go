package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultPlayerAPIKey    = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	defaultClientName      = "WEB"
	defaultClientVersion   = "2.20241108.08.00"
	defaultPlayerBaseURL   = "https://www.youtube.com/youtubei/v1/player"
	defaultPlayerUserAgent = "live-stream-alerts/1.0"
)

// PlayerClientOptions configures the YouTube player API client.
type PlayerClientOptions struct {
	APIKey        string
	ClientName    string
	ClientVersion string
	BaseURL       string
	HTTPClient    *http.Client
}

// PlayerClient fetches metadata about a specific YouTube video.
type PlayerClient struct {
	apiKey        string
	clientName    string
	clientVersion string
	baseURL       string
	httpClient    *http.Client
}

// NewPlayerClient builds a PlayerClient with the provided options.
func NewPlayerClient(opts PlayerClientOptions) *PlayerClient {
	apiKey := opts.APIKey
	if apiKey == "" {
		apiKey = defaultPlayerAPIKey
	}
	clientName := opts.ClientName
	if clientName == "" {
		clientName = defaultClientName
	}
	clientVersion := opts.ClientVersion
	if clientVersion == "" {
		clientVersion = defaultClientVersion
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultPlayerBaseURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	return &PlayerClient{
		apiKey:        apiKey,
		clientName:    clientName,
		clientVersion: clientVersion,
		baseURL:       baseURL,
		httpClient:    httpClient,
	}
}

// LiveStatus describes whether a video is a livestream and if it's online.
type LiveStatus struct {
	VideoID           string
	Title             string
	ChannelID         string
	IsLive            bool
	IsLiveNow         bool
	PlayabilityStatus string
}

// IsOnline reports whether the livestream appears to be running now.
func (s LiveStatus) IsOnline() bool {
	return s.IsLive && s.IsLiveNow && strings.EqualFold(s.PlayabilityStatus, "OK")
}

// LiveStatus fetches the player metadata for the supplied video ID.
func (c *PlayerClient) LiveStatus(ctx context.Context, videoID string) (LiveStatus, error) {
	payload := map[string]any{
		"videoId": videoID,
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    c.clientName,
				"clientVersion": c.clientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return LiveStatus{}, fmt.Errorf("encode player payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s?key=%s", c.baseURL, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return LiveStatus{}, fmt.Errorf("build player request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultPlayerUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return LiveStatus{}, fmt.Errorf("request player API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return LiveStatus{}, fmt.Errorf("player API %s: %s", resp.Status, string(preview))
	}

	var decoded playerResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return LiveStatus{}, fmt.Errorf("decode player response: %w", err)
	}

	status := LiveStatus{
		VideoID:           videoID,
		Title:             decoded.VideoDetails.Title,
		ChannelID:         decoded.VideoDetails.ChannelID,
		IsLive:            decoded.VideoDetails.IsLiveContent,
		PlayabilityStatus: decoded.PlayabilityStatus.Status,
	}
	if details := decoded.Microformat.PlayerMicroformatRenderer.LiveBroadcastDetails; details != nil {
		status.IsLiveNow = details.IsLiveNow
	}

	return status, nil
}

type playerResponse struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
	} `json:"playabilityStatus"`
	VideoDetails struct {
		Title         string `json:"title"`
		ChannelID     string `json:"channelId"`
		IsLiveContent bool   `json:"isLiveContent"`
	} `json:"videoDetails"`
	Microformat struct {
		PlayerMicroformatRenderer struct {
			LiveBroadcastDetails *struct {
				IsLiveNow bool `json:"isLiveNow"`
			} `json:"liveBroadcastDetails"`
		} `json:"playerMicroformatRenderer"`
	} `json:"microformat"`
}
