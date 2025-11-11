# Changelog

## [Unreleased]
### Added
- Introduced the v1 HTTP router with request-dump logging so every inbound request is captured alongside the YouTube alert verification endpoint.
- Added the `/api/v1/youtube/subscribe` proxy that forwards JSON payloads to the YouTube PubSubHubbub hub while applying the required defaults.
- Added the `/api/v1/youtube/channel` lookup endpoint to convert @handles into canonical UC channel IDs.
- Added POST `/api/v1/streamers` to persist streamer metadata into `data/streamers.json` for multi-platform support.
- Added GET `/api/v1/streamers` so clients can list every stored streamer record.
- Added the `web/algui` WebAssembly UI sources so the server can ship a minimal dashboard out of the box.
- Rebuilt the `web/algui` UI in Go WASM so the landing page now mirrors the original React roster (including styling, status badges, the SubmitStreamerForm, and a static fallback dataset).
- Added `streamer.description` to the schema and storage model so submissions can describe what makes each streamer unique.
- Derived `streamer.id` from the alias by stripping whitespace/punctuation and tightened the schema to enforce alphanumeric IDs.
- Reject duplicate streamer aliases by enforcing unique cleaned IDs during persistence and documenting the resulting `409 Conflict` behavior.
- The alGUI Submit Streamer form now POSTs directly to `/api/v1/streamers`, surfacing conflicts when an alias already exists.
- Added `streamer.languages` to the schema/storage plus validation so submissions only include supported language codes.
- Added `/api/v1/metadata/description` so the UI can fetch channel summaries and auto-fill the description field when a URL is entered.
- Added `web/README.md` so contributors know how to build and serve the alGUI assets.
- Added a JSON schema (`schema/streamers.schema.json`) and typed storage layer for streamers so data persists with server-managed IDs and timestamps.
- Stubbed platform folders (`internal/platforms/{youtube,facebook,twitch}`) plus shared logging utilities to support future providers.
- Added a root `.gitignore` to drop editor/OS cruft, `cmd/alertserver/out.bin`, and other generated artifacts (including `web/algui/main.wasm`).
- Added a root `README.md` with setup instructions and a canonical list of every HTTP endpoint so future additions stay documented.
### Changed
- The subscribe handler now mirrors the hub's HTTP response (body/status) to the API client and falls back to the upstream status text when the hub omits a body.
- Normalized all YouTube WebSub defaults (callback URL, lease duration, verification mode) inside the handler so clients can omit them safely.
- Alert verification logging now includes the exact challenge response body so the terminal reflects what was sent back to YouTube.
### Fixed
- Persist `streamer.alias` when creating records and require it as the primary identifier so requests without names no longer lose the alias field.
- Removed references to the deprecated `/api/v1/youtube/new/subscribe` alias so the README only lists active endpoints.
