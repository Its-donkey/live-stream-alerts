// file name — /config/config.go
package config

import (
	"encoding/json"
	"log"
	"os"
)

// Config models the structure stored in config.json (or similar overrides).
type Config struct {
	YouTube YouTubeConfig `json:"youtube"`
}

// YouTubeConfig captures the WebSub-specific defaults persisted in config files.
type YouTubeConfig struct {
	HubURL       string `json:"hub_url"`
	CallbackURL  string `json:"callback_url"`
	LeaseSeconds int    `json:"lease_seconds"`
	Mode         string `json:"mode"`
	Verify       string `json:"verify"`
}

var C Config // exported global config

func MustLoad(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &C); err != nil {
		log.Fatal(err)
	}
}
