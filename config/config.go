// file name — /config/config.go
package config

import (
	"encoding/json"
	"log"
	"os"
)

const (
	defaultAddr = "127.0.0.1"
	defaultPort = ":8880"
)

// YouTubeConfig captures the WebSub-specific defaults persisted in config files.
type YouTubeConfig struct {
	HubURL       string `json:"hub_url"`
	CallbackURL  string `json:"callback_url"`
	LeaseSeconds int    `json:"lease_seconds"`
	Mode         string `json:"mode"`
	Verify       string `json:"verify"`
}

// ServerConfig configures the HTTP listener used by alert-server.
type ServerConfig struct {
	Addr string `json:"addr"`
	Port string `json:"port"`
}

// Config represents the combined runtime settings parsed from config.json.
type Config struct {
	Server  ServerConfig
	YouTube YouTubeConfig
}

var (
	YT     YouTubeConfig
	Server ServerConfig
)

type fileConfig struct {
	ServerBlock  *ServerConfig  `json:"server"`
	Addr         string         `json:"addr"`
	Port         string         `json:"port"`
	YouTubeBlock *YouTubeConfig `json:"youtube"`
	YouTubeConfig
}

// MustLoad reads the JSON config at the given path, populates global defaults,
// and returns the parsed config structure.
func MustLoad(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var raw fileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Fatal(err)
	}

	yt := raw.YouTubeConfig
	if raw.YouTubeBlock != nil {
		yt = *raw.YouTubeBlock
	}

	server := ServerConfig{
		Addr: raw.Addr,
		Port: raw.Port,
	}
	if raw.ServerBlock != nil {
		server = *raw.ServerBlock
		if server.Addr == "" {
			server.Addr = raw.Addr
		}
		if server.Port == "" {
			server.Port = raw.Port
		}
	}
	if server.Addr == "" {
		server.Addr = defaultAddr
	}
	if server.Port == "" {
		server.Port = defaultPort
	}

	cfg := Config{
		Server:  server,
		YouTube: yt,
	}

	YT = cfg.YouTube
	Server = cfg.Server
	return cfg
}
