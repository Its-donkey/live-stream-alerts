package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"live-stream-alerts/internal/platforms/youtube/subscriptions"
)

// ChannelResolver resolves YouTube @handles into channel IDs.
type ChannelResolver struct {
	Client *http.Client
}

// ResolveHandle converts the provided handle into a canonical channel ID.
func (r ChannelResolver) ResolveHandle(ctx context.Context, handle string) (string, error) {
	trimmed := strings.TrimSpace(handle)
	if trimmed == "" {
		return "", fmt.Errorf("%w: handle is required", ErrValidation)
	}
	channelID, err := subscriptions.ResolveChannelID(ctx, trimmed, r.Client)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return channelID, nil
}
