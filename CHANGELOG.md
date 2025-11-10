# Changelog

## [Unreleased]
### Added
- Introduced the v1 HTTP router with request-dump logging so every inbound request is captured alongside the YouTube alert verification endpoint.
- Added the `/api/v1/youtube/subscribe` proxy that forwards JSON payloads to the YouTube PubSubHubbub hub while applying the required defaults.
- Added the `/api/v1/youtube/channel` lookup endpoint to convert @handles into canonical UC channel IDs.
- Added POST `/api/v1/streamers` to persist streamer metadata into `data/streamers.json` for multi-platform support.
- Added GET `/api/v1/streamers` so clients can list every stored streamer record.
- Added a JSON schema (`schema/streamers.schema.json`) and typed storage layer for streamers so data persists with server-managed IDs and timestamps.
- Stubbed platform folders (`internal/platforms/{youtube,facebook,twitch}`) plus shared logging utilities to support future providers.
- Added a root `README.md` with setup instructions and a canonical list of every HTTP endpoint so future additions stay documented.
### Changed
- The subscribe handler now mirrors the hub's HTTP response (body/status) to the API client and falls back to the upstream status text when the hub omits a body.
- Normalized all YouTube WebSub defaults (callback URL, lease duration, verification mode) inside the handler so clients can omit them safely.
- Alert verification logging now includes the exact challenge response body so the terminal reflects what was sent back to YouTube.
- Removed references to the deprecated `/api/v1/youtube/new/subscribe` alias so the README only lists active endpoints.
