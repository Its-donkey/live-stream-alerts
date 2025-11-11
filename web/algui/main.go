//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type platformFormRow struct {
	ID         string
	Name       string
	Preset     string
	ChannelURL string
}

type platformFieldError struct {
	Name    bool
	Channel bool
}

type submitFormErrors struct {
	Name        bool
	Description bool
	Languages   bool
	Platforms   map[string]platformFieldError
}

type submitFormState struct {
	Open          bool
	Name          string
	Description   string
	Languages     []string
	Platforms     []platformFormRow
	Errors        submitFormErrors
	ResultMessage string
	ResultState   string
	Submitting    bool
}

type createStreamerRequest struct {
	Streamer  streamerPayload   `json:"streamer"`
	Platforms streamerPlatforms `json:"platforms"`
}

type streamerPayload struct {
	Alias       string   `json:"alias"`
	Description string   `json:"description,omitempty"`
	Languages   []string `json:"languages,omitempty"`
}

type streamerPlatforms struct {
	YouTube  *platformYouTube  `json:"youtube,omitempty"`
	Facebook *platformFacebook `json:"facebook,omitempty"`
	Twitch   *platformTwitch   `json:"twitch,omitempty"`
}

type platformYouTube struct {
	Handle    string `json:"handle"`
	ChannelID string `json:"channelId,omitempty"`
}

type platformFacebook struct {
	PageID string `json:"pageId,omitempty"`
}

type platformTwitch struct {
	Username string `json:"username,omitempty"`
}

type createStreamerResponse struct {
	Streamer struct {
		ID    string `json:"id"`
		Alias string `json:"alias"`
	} `json:"streamer"`
}

var (
	document      = js.Global().Get("document")
	streamerTable js.Value
	refreshFunc   js.Func
	submitState   submitFormState
	formHandlers  []js.Func
)

const (
	maxLanguages = 8
	maxPlatforms = 8
)

var statusLabels = map[string]string{
	"online":  "Online",
	"busy":    "Workshop",
	"offline": "Offline",
}

type languageOption struct {
	Label string
	Value string
}

var languageOptions = []languageOption{
	{Label: "English", Value: "English"},
	{Label: "Afrikaans", Value: "Afrikaans"},
	{Label: "Albanian / Shqip", Value: "Albanian"},
	{Label: "Amharic / አማርኛ (Amariññā)", Value: "Amharic"},
	{Label: "Armenian / Հայերեն (Hayeren)", Value: "Armenian"},
	{Label: "Azerbaijani / Azərbaycanca", Value: "Azerbaijani"},
	{Label: "Basque / Euskara", Value: "Basque"},
	{Label: "Belarusian / Беларуская (Belaruskaya)", Value: "Belarusian"},
	{Label: "Bosnian / Bosanski", Value: "Bosnian"},
	{Label: "Bulgarian / Български (Bŭlgarski)", Value: "Bulgarian"},
	{Label: "Catalan / Català", Value: "Catalan"},
	{Label: "Cebuano / Binisaya", Value: "Cebuano"},
	{Label: "Croatian / Hrvatski", Value: "Croatian"},
	{Label: "Czech / Čeština", Value: "Czech"},
	{Label: "Danish / Dansk", Value: "Danish"},
	{Label: "Dutch / Nederlands", Value: "Dutch"},
	{Label: "Estonian / Eesti", Value: "Estonian"},
	{Label: "Filipino / Tagalog", Value: "Filipino"},
	{Label: "Finnish / Suomi", Value: "Finnish"},
	{Label: "Galician / Galego", Value: "Galician"},
	{Label: "Georgian / ქართული (Kartuli)", Value: "Georgian"},
	{Label: "German / Deutsch", Value: "German"},
	{Label: "Greek / Ελληνικά (Elliniká)", Value: "Greek"},
	{Label: "Gujarati / ગુજરાતી (Gujarātī)", Value: "Gujarati"},
	{Label: "Haitian Creole / Kreyòl Ayisyen", Value: "Haitian Creole"},
	{Label: "Hebrew / עברית (Ivrit)", Value: "Hebrew"},
	{Label: "Hmong / Hmoob", Value: "Hmong"},
	{Label: "Hungarian / Magyar", Value: "Hungarian"},
	{Label: "Icelandic / Íslenska", Value: "Icelandic"},
	{Label: "Igbo", Value: "Igbo"},
	{Label: "Italian / Italiano", Value: "Italian"},
	{Label: "Japanese / 日本語 (Nihongo)", Value: "Japanese"},
	{Label: "Javanese / Basa Jawa", Value: "Javanese"},
	{Label: "Kannada / ಕನ್ನಡ (Kannaḍa)", Value: "Kannada"},
	{Label: "Kazakh / Қазақ (Qazaq)", Value: "Kazakh"},
	{Label: "Khmer / ខ្មែរ (Khmer)", Value: "Khmer"},
	{Label: "Kinyarwanda", Value: "Kinyarwanda"},
	{Label: "Korean / 한국어 (Hangugeo)", Value: "Korean"},
	{Label: "Kurdish / کوردی", Value: "Kurdish"},
	{Label: "Lao / ລາວ", Value: "Lao"},
	{Label: "Latvian / Latviešu", Value: "Latvian"},
	{Label: "Lithuanian / Lietuvių", Value: "Lithuanian"},
	{Label: "Luxembourgish / Lëtzebuergesch", Value: "Luxembourgish"},
	{Label: "Macedonian / Македонски (Makedonski)", Value: "Macedonian"},
	{Label: "Malay / Bahasa Melayu", Value: "Malay"},
	{Label: "Malayalam / മലയാളം (Malayāḷam)", Value: "Malayalam"},
	{Label: "Maltese / Malti", Value: "Maltese"},
	{Label: "Marathi / मराठी (Marāṭhī)", Value: "Marathi"},
	{Label: "Mongolian / Монгол (Mongol)", Value: "Mongolian"},
	{Label: "Nepali / नेपाली (Nepālī)", Value: "Nepali"},
	{Label: "Norwegian / Norsk", Value: "Norwegian"},
	{Label: "Pashto / پښتو", Value: "Pashto"},
	{Label: "Persian / فارسی (Fārsi)", Value: "Persian"},
	{Label: "Polish / Polski", Value: "Polish"},
	{Label: "Punjabi / ਪੰਜਾਬੀ (Pañjābī)", Value: "Punjabi"},
	{Label: "Romanian / Română", Value: "Romanian"},
	{Label: "Serbian / Српски (Srpski)", Value: "Serbian"},
	{Label: "Sinhala / සිංහල (Siṁhala)", Value: "Sinhala"},
	{Label: "Slovak / Slovenčina", Value: "Slovak"},
	{Label: "Slovenian / Slovenščina", Value: "Slovenian"},
	{Label: "Somali / Soomaali", Value: "Somali"},
	{Label: "Swahili / Kiswahili", Value: "Swahili"},
	{Label: "Swedish / Svenska", Value: "Swedish"},
	{Label: "Tamil / தமிழ் (Tamiḻ)", Value: "Tamil"},
	{Label: "Telugu / తెలుగు (Telugu)", Value: "Telugu"},
	{Label: "Thai / ไทย", Value: "Thai"},
	{Label: "Turkish / Türkçe", Value: "Turkish"},
	{Label: "Ukrainian / Українська (Ukrayins'ka)", Value: "Ukrainian"},
	{Label: "Urdu / اردو", Value: "Urdu"},
	{Label: "Uzbek / Oʻzbek", Value: "Uzbek"},
	{Label: "Vietnamese / Tiếng Việt", Value: "Vietnamese"},
	{Label: "Welsh / Cymraeg", Value: "Welsh"},
	{Label: "Xhosa / isiXhosa", Value: "Xhosa"},
	{Label: "Yoruba / Yorùbá", Value: "Yoruba"},
	{Label: "Zulu / isiZulu", Value: "Zulu"},
}

var platformPresets = []string{
	"YouTube",
	"Twitch",
	"Facebook Live",
	"Instagram Live",
	"Kick",
	"TikTok Live",
	"Trovo",
	"Rumble",
	"Discord",
}

func main() {
	done := make(chan struct{})
	buildShell()
	initSubmitForm()
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
			if mapped := statusLabels[status]; mapped != "" {
				label = mapped
			} else if len(status) > 0 {
				label = strings.ToUpper(status[:1]) + status[1:]
			} else {
				label = "Offline"
			}
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

  <div id="submit-streamer-section"></div>
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

func initSubmitForm() {
	submitState = submitFormState{
		Platforms: []platformFormRow{newPlatformRow()},
		Errors: submitFormErrors{
			Platforms: make(map[string]platformFieldError),
		},
	}
	renderSubmitForm()
}

func renderSubmitForm() {
	container := document.Call("getElementById", "submit-streamer-section")
	if !container.Truthy() {
		return
	}

	focus := captureFocusSnapshot()
	releaseFormHandlers()

	if len(submitState.Platforms) == 0 {
		submitState.Platforms = []platformFormRow{newPlatformRow()}
	}
	if submitState.Errors.Platforms == nil {
		submitState.Errors.Platforms = make(map[string]platformFieldError)
	}

	var builder strings.Builder
	builder.WriteString(`<section class="submit-streamer" aria-labelledby="submit-streamer-title">`)
	builder.WriteString(`
  <div class="submit-streamer-header">
    <h2 id="submit-streamer-title">Know a streamer we should feature?</h2>
    <button type="button" class="submit-streamer-toggle" id="submit-toggle">`)
	if submitState.Open {
		builder.WriteString("Hide form")
	} else {
		builder.WriteString("Submit a streamer")
	}
	builder.WriteString(`</button>
  </div>`)

	if submitState.Open {
		formClass := "submit-streamer-form"
		if submitState.Submitting {
			formClass += " is-submitting"
		}
		builder.WriteString(`<form id="submit-streamer-form" class="` + formClass + `" aria-live="polite">`)
		builder.WriteString(`<p class="submit-streamer-help">Share the details below and our team will review the submission before adding the streamer to the roster. No additional access is required.</p>`)

		// Form grid
		builder.WriteString(`<div class="form-grid">`)
		// Name field
		nameClass := "form-field"
		if submitState.Errors.Name {
			nameClass += " form-field-error"
		}
		builder.WriteString(`<label class="` + nameClass + `" id="field-name"><span>Streamer name *</span><input type="text" id="streamer-name" value="` + html.EscapeString(submitState.Name) + `" required /></label>`)

		// Description
		descClass := "form-field form-field-wide"
		if submitState.Errors.Description {
			descClass += " form-field-error"
		}
		builder.WriteString(`<label class="` + descClass + `" id="field-description"><span>Description *</span><p class="submit-streamer-help">What does the streamer do and what makes their streams unique?</p><textarea id="streamer-description" rows="3" required>`)
		builder.WriteString(html.EscapeString(submitState.Description))
		builder.WriteString(`</textarea></label>`)

		// Languages
		langClass := "form-field form-field-wide"
		if submitState.Errors.Languages {
			langClass += " form-field-error"
		}
		builder.WriteString(`<label class="` + langClass + `" id="field-languages"><span>Languages *</span><p class="submit-streamer-help">Select every language the streamer uses on their channel.</p>`)
		selectDisabled := len(submitState.Languages) >= maxLanguages
		builder.WriteString(`<div class="language-picker">`)
		builder.WriteString(`<select class="language-select" id="language-select"`)
		if selectDisabled {
			builder.WriteString(` disabled`)
		}
		builder.WriteString(`>`)
		builder.WriteString(`<option value="">Languages</option>`)
		for _, option := range availableLanguageOptions(submitState.Languages) {
			builder.WriteString(`<option value="` + html.EscapeString(option.Value) + `">` + html.EscapeString(option.Label) + `</option>`)
		}
		builder.WriteString(`</select>`)
		builder.WriteString(`<div class="language-tags">`)
		if len(submitState.Languages) == 0 {
			builder.WriteString(`<span class="language-empty">No languages selected yet.</span>`)
		} else {
			for _, language := range submitState.Languages {
				builder.WriteString(`<span class="language-pill">` + html.EscapeString(language) + `<button type="button" data-remove-language="` + html.EscapeString(language) + `" aria-label="Remove ` + html.EscapeString(language) + `">×</button></span>`)
			}
		}
		builder.WriteString(`</div>`)
		builder.WriteString(`</div>`)
		if submitState.Errors.Languages {
			builder.WriteString(`<p class="field-error-text">Select at least one language.</p>`)
		}
		builder.WriteString(`</label></div>`) // end languages label and grid

		// Platform fieldset
		builder.WriteString(`<fieldset class="platform-fieldset"><legend>Streaming platforms *</legend><p class="submit-streamer-help">Add each platform’s name and channel URL. If they’re the same stream link, repeat the URL.</p>`)
		builder.WriteString(`<div class="platform-rows">`)
		for _, row := range submitState.Platforms {
			errors := submitState.Errors.Platforms[row.ID]
			nameWrapper := "form-field form-field-inline"
			if errors.Name {
				nameWrapper += " form-field-error"
			}
			channelWrapper := "form-field form-field-inline"
			if errors.Channel {
				channelWrapper += " form-field-error"
			}
			builder.WriteString(`<div class="platform-row" data-platform-row="` + row.ID + `">`)
			builder.WriteString(`<label class="` + nameWrapper + `" id="platform-name-field-` + row.ID + `"><span>Platform name</span><div class="platform-picker"><select class="platform-select" data-platform-select data-row="` + row.ID + `"><option value="">Choose platform</option>`)
			presetMatched := false
			for _, preset := range platformPresets {
				selected := ""
				if strings.EqualFold(row.Name, preset) {
					selected = " selected"
					presetMatched = true
				}
				builder.WriteString(`<option value="` + html.EscapeString(preset) + `"` + selected + `>` + html.EscapeString(preset) + `</option>`)
			}
			if row.Name != "" && !presetMatched {
				builder.WriteString(`<option value="` + html.EscapeString(row.Name) + `" selected>` + html.EscapeString(row.Name) + `</option>`)
			}
			builder.WriteString(`</select></div></label>`)

			builder.WriteString(`<label class="` + channelWrapper + `" id="platform-url-field-` + row.ID + `"><span>Channel URL</span><input type="url" placeholder="https://" value="` + html.EscapeString(row.ChannelURL) + `" data-platform-channel data-row="` + row.ID + `" required /></label>`)

			builder.WriteString(`<button type="button" class="remove-platform-button" data-remove-platform="` + row.ID + `">Remove</button>`)
			if errors.Name || errors.Channel {
				builder.WriteString(`<p class="field-error-text">Provide the platform name and channel URL.</p>`)
			}
			builder.WriteString(`</div>`)
		}
		builder.WriteString(`</div>`)

		addDisabled := ""
		if len(submitState.Platforms) >= maxPlatforms {
			addDisabled = " disabled"
		}
		builder.WriteString(`<button type="button" class="add-platform-button" id="add-platform"` + addDisabled + `>+ Add another platform</button>`)
		builder.WriteString(`</fieldset>`)

		// Actions
		builder.WriteString(`<div class="submit-streamer-actions">`)
		submitLabel := "Submit streamer"
		if submitState.Submitting {
			submitLabel = "Submitting…"
		}
		disableSubmit := ""
		if submitState.Submitting {
			disableSubmit = " disabled"
		}
		builder.WriteString(`<button type="submit" class="submit-streamer-submit"` + disableSubmit + `>` + submitLabel + `</button>`)
		builder.WriteString(`<button type="button" class="submit-streamer-cancel" id="submit-cancel">Cancel</button>`)
		builder.WriteString(`</div>`)

		if submitState.ResultState != "" && submitState.ResultMessage != "" {
			builder.WriteString(`<div class="submit-streamer-result" data-state="` + html.EscapeString(submitState.ResultState) + `" role="status">` + html.EscapeString(submitState.ResultMessage) + `</div>`)
		}

		builder.WriteString(`</form>`)
	}

	builder.WriteString(`</section>`)
	container.Set("innerHTML", builder.String())

	bindSubmitFormEvents()
	restoreFocusSnapshot(focus)
}

type focusSnapshot struct {
	ID    string
	Start int
	End   int
}

func captureFocusSnapshot() focusSnapshot {
	active := document.Get("activeElement")
	if !active.Truthy() {
		return focusSnapshot{Start: -1, End: -1}
	}
	idValue := active.Get("id")
	if idValue.Type() != js.TypeString {
		return focusSnapshot{Start: -1, End: -1}
	}
	snap := focusSnapshot{ID: idValue.String(), Start: -1, End: -1}
	if start := active.Get("selectionStart"); start.Type() == js.TypeNumber {
		snap.Start = start.Int()
	}
	if end := active.Get("selectionEnd"); end.Type() == js.TypeNumber {
		snap.End = end.Int()
	}
	return snap
}

func restoreFocusSnapshot(snap focusSnapshot) {
	if snap.ID == "" {
		return
	}
	target := document.Call("getElementById", snap.ID)
	if !target.Truthy() {
		return
	}
	target.Call("focus")
	if snap.Start >= 0 && snap.End >= 0 {
		if setter := target.Get("setSelectionRange"); setter.Type() == js.TypeFunction {
			target.Call("setSelectionRange", snap.Start, snap.End)
		}
	}
}

func bindSubmitFormEvents() {
	toggle := document.Call("getElementById", "submit-toggle")
	addFormHandler(toggle, "click", func(js.Value, []js.Value) any {
		submitState.Open = !submitState.Open
		if !submitState.Open {
			resetFormState(true)
		}
		renderSubmitForm()
		return nil
	})

	if !submitState.Open {
		return
	}

	nameInput := document.Call("getElementById", "streamer-name")
	addFormHandler(nameInput, "input", func(this js.Value, _ []js.Value) any {
		value := this.Get("value").String()
		submitState.Name = value
		if strings.TrimSpace(value) != "" {
			submitState.Errors.Name = false
			markFieldError("field-name", false)
		}
		return nil
	})

	descInput := document.Call("getElementById", "streamer-description")
	addFormHandler(descInput, "input", func(this js.Value, _ []js.Value) any {
		value := this.Get("value").String()
		submitState.Description = value
		if strings.TrimSpace(value) != "" {
			submitState.Errors.Description = false
			markFieldError("field-description", false)
		}
		return nil
	})

	langSelect := document.Call("getElementById", "language-select")
	addFormHandler(langSelect, "change", func(this js.Value, _ []js.Value) any {
		value := strings.TrimSpace(this.Get("value").String())
		if value == "" {
			return nil
		}
		if len(submitState.Languages) >= maxLanguages {
			return nil
		}
		label := languageLabelByValue[value]
		if label == "" {
			label = value
		}
		if !containsString(submitState.Languages, label) {
			submitState.Languages = append(submitState.Languages, label)
			submitState.Errors.Languages = false
		}
		renderSubmitForm()
		return nil
	})

	langButtons := document.Call("querySelectorAll", "[data-remove-language]")
	forEachNode(langButtons, func(node js.Value) {
		addFormHandler(node, "click", func(this js.Value, _ []js.Value) any {
			language := this.Get("dataset").Get("removeLanguage").String()
			if language == "" {
				return nil
			}
			filtered := make([]string, 0, len(submitState.Languages))
			for _, entry := range submitState.Languages {
				if entry != language {
					filtered = append(filtered, entry)
				}
			}
			submitState.Languages = filtered
			if len(filtered) > 0 {
				submitState.Errors.Languages = false
			}
			renderSubmitForm()
			return nil
		})
	})

	platformSelects := document.Call("querySelectorAll", "[data-platform-select]")
	forEachNode(platformSelects, func(node js.Value) {
		addFormHandler(node, "change", func(this js.Value, _ []js.Value) any {
			rowID := this.Get("dataset").Get("row").String()
			value := this.Get("value").String()
			for index, row := range submitState.Platforms {
				if row.ID == rowID {
					submitState.Platforms[index].Name = value
					submitState.Platforms[index].Preset = value
					if strings.TrimSpace(value) != "" {
						clearPlatformError(rowID, "name")
					}
					break
				}
			}
			return nil
		})
	})

	platformInputs := document.Call("querySelectorAll", "[data-platform-channel]")
	forEachNode(platformInputs, func(node js.Value) {
		addFormHandler(node, "input", func(this js.Value, _ []js.Value) any {
			rowID := this.Get("dataset").Get("row").String()
			value := this.Get("value").String()
			for index, row := range submitState.Platforms {
				if row.ID == rowID {
					submitState.Platforms[index].ChannelURL = value
					if strings.TrimSpace(value) != "" {
						clearPlatformError(rowID, "channel")
					}
					break
				}
			}
			return nil
		})
	})

	removeButtons := document.Call("querySelectorAll", "[data-remove-platform]")
	forEachNode(removeButtons, func(node js.Value) {
		addFormHandler(node, "click", func(this js.Value, _ []js.Value) any {
			rowID := this.Get("dataset").Get("removePlatform").String()
			if rowID == "" {
				return nil
			}
			if len(submitState.Platforms) == 1 {
				submitState.Platforms = []platformFormRow{newPlatformRow()}
				submitState.Errors.Platforms = make(map[string]platformFieldError)
			} else {
				next := make([]platformFormRow, 0, len(submitState.Platforms)-1)
				for _, row := range submitState.Platforms {
					if row.ID != rowID {
						next = append(next, row)
					}
				}
				submitState.Platforms = next
			}
			if submitState.Errors.Platforms != nil {
				delete(submitState.Errors.Platforms, rowID)
			}
			renderSubmitForm()
			return nil
		})
	})

	addButton := document.Call("getElementById", "add-platform")
	addFormHandler(addButton, "click", func(js.Value, []js.Value) any {
		if len(submitState.Platforms) >= maxPlatforms {
			return nil
		}
		submitState.Platforms = append(submitState.Platforms, newPlatformRow())
		renderSubmitForm()
		return nil
	})

	cancelBtn := document.Call("getElementById", "submit-cancel")
	addFormHandler(cancelBtn, "click", func(js.Value, []js.Value) any {
		resetFormState(true)
		submitState.Open = false
		renderSubmitForm()
		return nil
	})

	form := document.Call("getElementById", "submit-streamer-form")
	addFormHandler(form, "submit", func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			args[0].Call("preventDefault")
		}
		handleSubmit()
		return nil
	})
}

func addFormHandler(node js.Value, event string, handler func(js.Value, []js.Value) any) {
	if !node.Truthy() {
		return
	}
	fn := js.FuncOf(handler)
	node.Call("addEventListener", event, fn)
	formHandlers = append(formHandlers, fn)
}

func releaseFormHandlers() {
	for _, fn := range formHandlers {
		fn.Release()
	}
	formHandlers = formHandlers[:0]
}

func forEachNode(list js.Value, fn func(js.Value)) {
	if !list.Truthy() {
		return
	}
	length := list.Get("length").Int()
	for i := 0; i < length; i++ {
		fn(list.Index(i))
	}
}

func markFieldError(fieldID string, hasError bool) {
	field := document.Call("getElementById", fieldID)
	if !field.Truthy() {
		return
	}
	classList := field.Get("classList")
	if hasError {
		classList.Call("add", "form-field-error")
	} else {
		classList.Call("remove", "form-field-error")
	}
}

func clearPlatformError(rowID, field string) {
	if submitState.Errors.Platforms == nil {
		submitState.Errors.Platforms = make(map[string]platformFieldError)
	}
	platformErr, ok := submitState.Errors.Platforms[rowID]
	if !ok {
		return
	}
	switch field {
	case "name":
		platformErr.Name = false
	case "channel":
		platformErr.Channel = false
	}
	if !platformErr.Name && !platformErr.Channel {
		delete(submitState.Errors.Platforms, rowID)
	} else {
		submitState.Errors.Platforms[rowID] = platformErr
	}
	markFieldError("platform-name-field-"+rowID, platformErr.Name)
	markFieldError("platform-url-field-"+rowID, platformErr.Channel)
}

func handleSubmit() {
	if submitState.Submitting {
		return
	}

	valid := validateSubmission()
	if !valid {
		renderSubmitForm()
		return
	}

	trimmedName := strings.TrimSpace(submitState.Name)
	trimmedDescription := strings.TrimSpace(submitState.Description)
	description := buildStreamerDescription(trimmedDescription, submitState.Platforms)
	payload := createStreamerRequest{
		Streamer: streamerPayload{
			Alias:       trimmedName,
			Description: description,
			Languages:   append([]string(nil), submitState.Languages...),
		},
	}

	submitState.Submitting = true
	submitState.ResultState = ""
	submitState.ResultMessage = ""
	renderSubmitForm()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		message, err := submitStreamerRequest(ctx, payload)
		if err != nil {
			submitState.Submitting = false
			submitState.ResultState = "error"
			submitState.ResultMessage = err.Error()
			renderSubmitForm()
			return
		}

		submitState.Submitting = false
		submitState.ResultState = "success"
		if message == "" {
			message = "Submission received and queued for review."
		}
		submitState.ResultMessage = message
		clearFormFields()
		renderSubmitForm()
	}()
}

func validateSubmission() bool {
	errors := submitFormErrors{
		Platforms: make(map[string]platformFieldError),
	}
	if strings.TrimSpace(submitState.Name) == "" {
		errors.Name = true
	}
	if strings.TrimSpace(submitState.Description) == "" {
		errors.Description = true
	}
	if len(submitState.Languages) == 0 {
		errors.Languages = true
	}

	for _, row := range submitState.Platforms {
		rowErr := platformFieldError{
			Name:    strings.TrimSpace(row.Name) == "",
			Channel: strings.TrimSpace(row.ChannelURL) == "",
		}
		if rowErr.Name || rowErr.Channel {
			errors.Platforms[row.ID] = rowErr
		}
	}

	submitState.Errors = errors
	return !(errors.Name || errors.Description || errors.Languages || len(errors.Platforms) > 0)
}

func submitStreamerRequest(ctx context.Context, payload createStreamerRequest) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/streamers", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
			return "", errors.New(trimmed)
		}
		return "", fmt.Errorf("submission failed: %s", resp.Status)
	}

	var created createStreamerResponse
	if err := json.Unmarshal(body, &created); err != nil {
		return "Streamer submitted successfully.", nil
	}
	alias := strings.TrimSpace(created.Streamer.Alias)
	id := strings.TrimSpace(created.Streamer.ID)
	switch {
	case alias != "" && id != "":
		return fmt.Sprintf("%s added with ID %s.", alias, id), nil
	case alias != "":
		return fmt.Sprintf("%s added to the roster.", alias), nil
	default:
		return "Streamer submitted successfully.", nil
	}
}

func buildStreamerDescription(description string, platforms []platformFormRow) string {
	desc := strings.TrimSpace(description)
	platformSummary := formatPlatformSummary(platforms)
	switch {
	case desc != "" && platformSummary != "":
		return desc + "\n\nPlatforms: " + platformSummary
	case desc != "":
		return desc
	case platformSummary != "":
		return "Platforms: " + platformSummary
	default:
		return ""
	}
}

func formatPlatformSummary(platforms []platformFormRow) string {
	var parts []string
	for _, row := range platforms {
		name := strings.TrimSpace(row.Name)
		url := strings.TrimSpace(row.ChannelURL)
		switch {
		case name != "" && url != "":
			parts = append(parts, fmt.Sprintf("%s (%s)", name, url))
		case name != "":
			parts = append(parts, name)
		case url != "":
			parts = append(parts, url)
		}
	}
	return strings.Join(parts, ", ")
}

func clearFormFields() {
	submitState.Name = ""
	submitState.Description = ""
	submitState.Languages = nil
	submitState.Platforms = []platformFormRow{newPlatformRow()}
	submitState.Errors = submitFormErrors{
		Platforms: make(map[string]platformFieldError),
	}
}

func resetFormState(includeResult bool) {
	clearFormFields()
	submitState.Submitting = false
	if includeResult {
		submitState.ResultMessage = ""
		submitState.ResultState = ""
	}
}

func newPlatformRow() platformFormRow {
	return platformFormRow{
		ID:         fmt.Sprintf("platform-%d", time.Now().UnixNano()),
		Name:       "",
		Preset:     "",
		ChannelURL: "",
	}
}

func availableLanguageOptions(selected []string) []languageOption {
	options := make([]languageOption, 0, len(languageOptions))
	for _, option := range languageOptions {
		if !containsString(selected, option.Label) {
			options = append(options, option)
		}
	}
	return options
}

func containsString(list []string, target string) bool {
	for _, entry := range list {
		if entry == target {
			return true
		}
	}
	return false
}

var languageLabelByValue = func() map[string]string {
	values := make(map[string]string, len(languageOptions))
	for _, option := range languageOptions {
		values[option.Value] = option.Label
	}
	return values
}()
