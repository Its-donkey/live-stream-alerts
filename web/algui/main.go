//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"syscall/js"
	"time"
)

type platform struct {
	Name       string `json:"name"`
	ChannelURL string `json:"channelUrl"`
}

type streamer struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	StatusLabel string     `json:"statusLabel"`
	Languages   []string   `json:"languages"`
	Platforms   []platform `json:"platforms"`
}

type wrappedStreamers struct {
	Streamers []streamer `json:"streamers"`
}

var (
	document      = js.Global().Get("document")
	streamerTable js.Value
	refreshFunc   js.Func
)

func main() {
	done := make(chan struct{})
	buildShell()
	bindRefreshHandler()
	go refreshRoster()
	<-done
}

func buildShell() {
	root := document.Call("getElementById", "app-root")
	if !root.Truthy() {
		js.Global().Get("console").Call("error", "app root missing")
		return
	}

	root.Set("innerHTML", mainLayout())
	streamerTable = document.Call("getElementById", "streamer-rows")
}

func bindRefreshHandler() {
	if refreshFunc.Type() != js.TypeUndefined {
		refreshFunc.Release()
	}
	refreshFunc = js.FuncOf(func(this js.Value, args []js.Value) any {
		go refreshRoster()
		return nil
	})

	button := document.Call("getElementById", "refresh-roster")
	if button.Truthy() {
		button.Call("addEventListener", "click", refreshFunc)
	}
}

func refreshRoster() {
	setStatusRow("Loading streamer roster…", false)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	streamers, err := fetchStreamers(ctx)
	if err != nil {
		setStatusRow(fmt.Sprintf("Unable to load roster: %v", err), true)
		return
	}

	renderStreamers(streamers)
}

func fetchStreamers(ctx context.Context) ([]streamer, error) {
	endpoints := []string{
		"/api/streamers",
		"/api/v1/streamers",
		"streamers.json",
	}

	client := &http.Client{Timeout: 8 * time.Second}

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}

		if data, err := decodeStreamers(body); err == nil {
			return data, nil
		}
	}

	return fallbackStreamers(), nil
}

func decodeStreamers(payload []byte) ([]streamer, error) {
	var direct []streamer
	if err := json.Unmarshal(payload, &direct); err == nil && direct != nil {
		return direct, nil
	}

	var wrapped wrappedStreamers
	if err := json.Unmarshal(payload, &wrapped); err == nil {
		return wrapped.Streamers, nil
	}

	return nil, fmt.Errorf("unexpected response shape")
}

func renderStreamers(streamers []streamer) {
	if !streamerTable.Truthy() {
		return
	}

	if len(streamers) == 0 {
		setStatusRow("No streamers available at the moment.", false)
		return
	}

	var builder strings.Builder
	for _, s := range streamers {
		status := strings.ToLower(strings.TrimSpace(s.Status))
		if status == "" {
			status = "offline"
		}
		label := s.StatusLabel
		if strings.TrimSpace(label) == "" {
			label = strings.Title(status)
		}

		builder.WriteString("<tr>")
		builder.WriteString(`<td data-label="Status"><span class="status ` + html.EscapeString(status) + `">`)
		builder.WriteString(html.EscapeString(label))
		builder.WriteString("</span></td>")

		builder.WriteString(`<td data-label="Name"><strong>`)
		builder.WriteString(html.EscapeString(s.Name))
		builder.WriteString("</strong>")
		if strings.TrimSpace(s.Description) != "" {
			builder.WriteString(`<div class="streamer-description">`)
			builder.WriteString(html.EscapeString(s.Description))
			builder.WriteString("</div>")
		}
		builder.WriteString("</td>")

		builder.WriteString(`<td data-label="Streaming Platforms">`)
		if len(s.Platforms) == 0 {
			builder.WriteString("—")
		} else {
			builder.WriteString(`<ul class="platform-list">`)
			for _, p := range s.Platforms {
				name := html.EscapeString(p.Name)
				url := strings.TrimSpace(p.ChannelURL)
				builder.WriteString("<li>")
				if url != "" {
					builder.WriteString(`<a class="platform-link" href="` + html.EscapeString(url) + `" target="_blank" rel="noopener noreferrer">` + name + `</a>`)
				} else {
					builder.WriteString(`<span class="platform-link" aria-disabled="true">` + name + `</span>`)
				}
				builder.WriteString("</li>")
			}
			builder.WriteString("</ul>")
		}
		builder.WriteString("</td>")

		builder.WriteString(`<td data-label="Language"><span class="lang">`)
		if len(s.Languages) == 0 {
			builder.WriteString("—")
		} else {
			builder.WriteString(html.EscapeString(strings.Join(s.Languages, " · ")))
		}
		builder.WriteString("</span></td>")
		builder.WriteString("</tr>")
	}

	streamerTable.Set("innerHTML", builder.String())
}

func setStatusRow(message string, allowRetry bool) {
	if !streamerTable.Truthy() {
		return
	}

	var builder strings.Builder
	builder.WriteString(`<tr><td colspan="4" class="table-status">`)
	builder.WriteString(html.EscapeString(message))
	if allowRetry {
		builder.WriteString(`<br/><button type="button" class="refresh-button" id="retry-fetch">Try again</button>`)
	}
	builder.WriteString("</td></tr>")

	streamerTable.Set("innerHTML", builder.String())

	if allowRetry {
		button := document.Call("getElementById", "retry-fetch")
		if button.Truthy() {
			button.Call("addEventListener", "click", refreshFunc)
		}
	}
}

func mainLayout() string {
	currentYear := time.Now().Year()
	return fmt.Sprintf(`
<div class="surface site-header">
  <div class="logo-lockup">
    <div class="logo-icon" aria-hidden="true">
      <svg viewBox="0 0 120 120" role="img" aria-labelledby="sharpen-logo-title">
        <title id="sharpen-logo-title">Sharpen Live logo</title>
        <defs>
          <linearGradient id="bladeGradient" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
            <stop offset="0%%" stop-color="#f8fafc" stop-opacity="0.95" />
            <stop offset="55%%" stop-color="#cbd5f5" stop-opacity="0.85" />
            <stop offset="100%%" stop-color="#7dd3fc" stop-opacity="0.95" />
          </linearGradient>
        </defs>
        <path d="M14 68c12-20 38-54 80-58l6 36c-12 6-26 14-41 26l-45-4z" fill="url(#bladeGradient)" stroke="#0f172a" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"></path>
        <path d="M19 76l35 4c-5 5-10 11-15 18l-26-8 6-14z" fill="rgba(15, 23, 42, 0.45)" stroke="#0f172a" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"></path>
        <circle cx="32" cy="92" r="6" fill="#38bdf8"></circle>
        <circle cx="88" cy="36" r="6" fill="#38bdf8"></circle>
      </svg>
    </div>
    <div class="logo-text">
      <h1>Sharpen.Live</h1>
      <p>Streaming Knife Craftsmen</p>
    </div>
  </div>
  <div class="header-actions">
    <a class="cta" href="#streamers">Become a Partner</a>
    <button type="button" class="admin-button" id="refresh-roster">Refresh roster</button>
  </div>
</div>

<main class="surface" id="streamers" aria-labelledby="streamers-title">
  <section class="intro">
    <h2 id="streamers-title">Live Knife Sharpening Studio</h2>
    <p>
      Discover bladesmiths and sharpening artists streaming in real time. Status indicators show who is live, who is prepping off camera, and who is offline.
      Premium partners share their booking links so you can send in your knives for a professional edge.
    </p>
  </section>

  <section class="streamer-table" aria-label="Sharpen Live streamer roster">
    <table>
      <thead>
        <tr>
          <th scope="col">Status</th>
          <th scope="col">Name</th>
          <th scope="col">Streaming Platforms</th>
          <th scope="col">Language</th>
        </tr>
      </thead>
      <tbody id="streamer-rows"></tbody>
    </table>
  </section>

  <section class="submit-banner">
    <h2>Want to join the roster?</h2>
    <p>
      Tell us about your sharpening studio, preferred streaming platforms, and availability. Our team reviews every submission to keep the directory curated.
      Email <a href="mailto:partners@sharpen.live">partners@sharpen.live</a> to get started.
    </p>
  </section>
</main>

<footer>
  <span>&copy; %d Sharpen Live. All rights reserved.</span>
  <span>Need assistance? <a href="mailto:hello@sharpen.live">hello@sharpen.live</a></span>
</footer>
`, currentYear)
}

func fallbackStreamers() []streamer {
	return []streamer{
		{
			ID:          "edgecrafter",
			Name:        "EdgeCrafter",
			Description: "Specializes in chef knives · Est. wait time 10 min",
			Status:      "online",
			StatusLabel: "Online",
			Languages:   []string{"English"},
			Platforms: []platform{
				{Name: "Twitch", ChannelURL: "https://www.twitch.tv/edgecrafter"},
				{Name: "YouTube", ChannelURL: "https://www.youtube.com/@edgecrafter"},
			},
		},
		{
			ID:          "zen-edge",
			Name:        "Zen Edge Studio",
			Description: "Waterstone specialist · Accepting rush orders",
			Status:      "busy",
			StatusLabel: "Workshop",
			Languages:   []string{"English", "Japanese"},
			Platforms: []platform{
				{Name: "YouTube", ChannelURL: "https://www.youtube.com/@zenedgestudio"},
			},
		},
		{
			ID:          "forge-feather",
			Name:        "Forge & Feather",
			Description: "Damascus patterns · Next stream 19:00 UTC",
			Status:      "offline",
			StatusLabel: "Offline",
			Languages:   []string{"French"},
			Platforms: []platform{
				{Name: "Kick", ChannelURL: "https://kick.com/forgeandfeather"},
				{Name: "Twitch", ChannelURL: "https://www.twitch.tv/forgeandfeather"},
			},
		},
		{
			ID:          "honbazuke",
			Name:        "Honbazuke Pro",
			Description: "Premium partners · Bookings open",
			Status:      "online",
			StatusLabel: "Online",
			Languages:   []string{"English", "German"},
			Platforms: []platform{
				{Name: "Twitch", ChannelURL: "https://www.twitch.tv/honbazukepro"},
				{Name: "Instagram Live", ChannelURL: "https://www.instagram.com/honbazukepro/"},
			},
		},
		{
			ID:          "sharp-true",
			Name:        "Sharp & True",
			Description: "Mobile service · On-site events available",
			Status:      "offline",
			StatusLabel: "Offline",
			Languages:   []string{"Spanish"},
			Platforms: []platform{
				{Name: "YouTube", ChannelURL: "https://www.youtube.com/@sharpandtrue"},
				{Name: "Facebook Live", ChannelURL: "https://www.facebook.com/sharpandtrue/live"},
			},
		},
	}
}
