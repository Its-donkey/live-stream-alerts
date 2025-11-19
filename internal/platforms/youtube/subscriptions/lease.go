// file name: internal/platforms/youtube/store/lease.go
package subscriptions

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"live-stream-alerts/internal/streamers"
)

// RecordLease stores the verification timestamp for the supplied channel ID.
func RecordLease(store *streamers.Store, channelID string, verifiedAt time.Time) error {
 channelID = strings.TrimSpace(channelID)
 if channelID == "" {
     return errors.New("channelID is required")
 }
 if store == nil {
     store = streamers.NewStore(streamers.DefaultFilePath)
 }

 err := store.UpdateFile(func(file *streamers.File) error {
		for i := range file.Records {
			yt := file.Records[i].Platforms.YouTube
			if yt == nil {
				continue
			}
			if strings.EqualFold(yt.ChannelID, channelID) {
				yt.HubLeaseDate = verifiedAt.UTC().Format(time.RFC3339)
				file.Records[i].UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return fmt.Errorf("channel id %s not found", channelID)
	})
	if err != nil {
		return fmt.Errorf("update streamers file: %w", err)
	}
	return nil
}
