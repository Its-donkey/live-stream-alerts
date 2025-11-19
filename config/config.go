// Package config loads and normalises alert-server configuration files.
package config

import (
	"encoding/json"
	"fmt"
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

// AdminConfig stores credentials for admin-authenticated APIs.
type AdminConfig struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	TokenTTLSeconds int    `json:"token_ttl_seconds"`
}

// Config represents the combined runtime settings parsed from config.json.
type Config struct {
	Server  ServerConfig
	YouTube YouTubeConfig
	Admin   AdminConfig
}

type fileConfig struct {
	ServerBlock  *ServerConfig  `json:"server"`
	Addr         string         `json:"addr"`
	Port         string         `json:"port"`
	YouTubeBlock *YouTubeConfig `json:"youtube"`
	YouTubeConfig
	AdminBlock *AdminConfig `json:"admin"`
	AdminConfig
}

// Load reads the JSON config at the given path and returns the parsed structure.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var raw fileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
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

	admin := raw.AdminConfig
	if raw.AdminBlock != nil {
		admin = *raw.AdminBlock
	}
	if admin.TokenTTLSeconds <= 0 {
		admin.TokenTTLSeconds = 86400
	}

	cfg := Config{
		Server:  server,
		YouTube: yt,
		Admin:   admin,
	}

	return cfg, nil
}

// MustLoad is a convenience wrapper around Load that panics on error.
func MustLoad(path string) Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}
