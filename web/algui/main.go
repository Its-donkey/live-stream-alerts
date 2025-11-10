//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"syscall/js"
	"time"
)

type serverConfig struct {
	Name        string `json:"name"`
	Addr        string `json:"addr"`
	Port        string `json:"port"`
	ReadTimeout string `json:"readTimeout"`
}

func main() {
	done := make(chan struct{})

	go func() {
		if err := render(); err != nil {
			notifyFailure(err)
		}
	}()

	<-done
}

func render() error {
	setStatus("Loading server configuration...")

	cfg, err := fetchConfig()
	if err != nil {
		return err
	}

	setStatus("alGUI connected")
	updateConfigList(cfg)
	return nil
}

func fetchConfig() (serverConfig, error) {
	var cfg serverConfig

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("/api/v1/server/config")
	if err != nil {
		return cfg, fmt.Errorf("request config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cfg, fmt.Errorf("config endpoint returned %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}

func updateConfigList(cfg serverConfig) {
	doc := js.Global().Get("document")
	list := doc.Call("getElementById", "config-list")
	if !list.Truthy() {
		return
	}

	html := fmt.Sprintf(`
        <li><span class="label">Name</span><span class="value">%s</span></li>
        <li><span class="label">Address</span><span class="value">%s</span></li>
        <li><span class="label">Port</span><span class="value">%s</span></li>
        <li><span class="label">Read Timeout</span><span class="value">%s</span></li>
    `, cfg.Name, cfg.Addr, cfg.Port, cfg.ReadTimeout)

	list.Set("innerHTML", html)
}

func setStatus(msg string) {
	doc := js.Global().Get("document")
	el := doc.Call("getElementById", "status")
	if !el.Truthy() {
		return
	}
	el.Set("textContent", msg)
}

func notifyFailure(err error) {
	setStatus("Failed to load server configuration")
	console := js.Global().Get("console")
	if console.Truthy() {
		console.Call("error", err.Error())
	}
}
