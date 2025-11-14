# Changelog

## [Unreleased]
### Added
- Introduced the v1 HTTP router with request-dump logging so every inbound request is captured alongside the YouTube alert verification endpoint.
- Added the `/api/v1/youtube/subscribe` proxy that forwards JSON payloads to the YouTube PubSubHubbub hub while applying the required defaults.
- Added the `/api/v1/youtube/channel` lookup endpoint to convert @handles into canonical UC channel IDs.
- Added POST `/api/v1/streamers` to persist streamer metadata into `data/streamers.json` for multi-platform support.
- Added GET `/api/v1/streamers` so clients can list every stored streamer record.
- Added `streamer.description` to the schema and storage model so submissions can describe what makes each streamer unique.
- Derived `streamer.id` from the alias by stripping whitespace/punctuation and tightened the schema to enforce alphanumeric IDs.
- Reject duplicate streamer aliases by enforcing unique cleaned IDs during persistence and documenting the resulting `409 Conflict` behavior.
- Added `/api/v1/metadata/description` so tooling can fetch channel summaries and auto-fill the description/name/YouTube handle fields when a URL is entered.
- Added `streamer.languages` to the schema/storage plus validation so submissions only include supported language codes.
- Automatically subscribes YouTube channels (via PubSubHubbub) whenever a newly created streamer includes YouTube platform data, resolving channel IDs from handles when needed.
- Added a JSON schema (`schema/streamers.schema.json`) and typed storage layer for streamers so data persists with server-managed IDs and timestamps.
- Stubbed platform folders (`internal/platforms/{youtube,facebook,twitch}`) plus shared logging utilities to support future providers.
- Added a root `.gitignore` to drop editor/OS cruft, `cmd/alertserver/out.bin`, and other generated artifacts (including generated WebAssembly binaries).
- Added a root `README.md` with setup instructions and a canonical list of every HTTP endpoint so future additions stay documented.
### Changed
- Moved the HTTP router under `internal/api/v1` and updated docs/CLI tooling so future endpoints live under their API versioned package.
- Relocated metadata scraping into the YouTube platform tree and corralled all YouTube handlers/clients/subscribers beneath `internal/platforms/youtube/{api,metadata,store,subscriptions}` for clearer ownership.
- Simplified `POST /api/v1/streamers` to accept only alias/description/languages plus a single YouTube channel URL, deriving the streamer ID, resolving channel metadata, generating a hub secret, updating the store, and triggering subscriptions automatically.
- Updated `DELETE /api/v1/streamers/{id}` to require both the matching path parameter and a JSON body containing the `streamer.id` and original `createdAt` timestamp, ensuring accidental deletions are caught before records are removed.
- Added dedicated GET/POST/DELETE handler coverage for `/api/v1/streamers` and now advertise all supported methods via the `Allow` header (including `DELETE`) so clients can reliably introspect the endpoint.
- Extracted the WebAssembly UI into a sibling project so this repository now focuses solely on the alert server APIs.
- The subscribe handler now mirrors the hub's HTTP response (body/status) to the API client and falls back to the upstream status text when the hub omits a body.
- Normalized all YouTube WebSub defaults (callback URL, lease duration, verification mode) inside the handler so clients can omit them safely.
- Alert verification logging now includes the exact challenge response body so the terminal reflects what was sent back to YouTube.
- Accepts `/alert` as an alias for `/alerts` so PubSubHubBub callbacks from older reverse-proxy configs are handled correctly.
- Fixed the router and verification handler so both `/alert` and `/alerts` paths are actually registered, preventing 404s when Google hits the legacy plural route.
- Expanded YouTube hub verification logging to include the full HTTP dump and planned response so challenges can be reviewed before they’re sent.
- Issued unique `hub.verify_token` values for every subscription and reject hub challenges whose topic/token/lease don’t match what was registered (mirroring the configured HMAC secret).
- Consolidated all logging through the internal logger package so runtime output shares consistent formatting regardless of entry point, including a blank spacer line before every timestamped entry for readability.
- Added explicit logging after sending the hub challenge reply so the status/body echoed back to YouTube are captured.
### Fixed
- Persist `streamer.alias` when creating records and require it as the primary identifier so requests without names no longer lose the alias field.
- Removed references to the deprecated `/api/v1/youtube/new/subscribe` alias so the README only lists active endpoints.
- Allow `DELETE /api/v1/streamers/{id}` to accept RFC3339 timestamps with or without fractional seconds so clients can resend the stored `createdAt` value without losing precision.
