// file name — /config/config.go
package config

import (
	"encoding/json"
	"log"
	"os"
)

// YouTubeConfig captures the WebSub-specific defaults persisted in config files.
type YouTubeConfig struct {
	HubURL       string `json:"hub_url"`
	CallbackURL  string `json:"callback_url"`
	LeaseSeconds int    `json:"lease_seconds"`
	Mode         string `json:"mode"`
	Verify       string `json:"verify"`
	DataAPIKey   string `json:"data_api_key"`
}

var YT YouTubeConfig // exported global config

func MustLoad(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &YT); err != nil {
		log.Fatal(err)
	}
	if apiKey := os.Getenv("YOUTUBE_DATA_API_KEY"); apiKey != "" {
		YT.DataAPIKey = apiKey
	}
}
