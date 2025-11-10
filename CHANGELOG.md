# Changelog

## [Unreleased]
### Added
- Introduced the v1 HTTP router with request-dump logging so every inbound request is captured alongside the YouTube alert verification endpoint.
- Added the `/api/V1/youtube/subscribe` proxy that forwards JSON payloads to the YouTube PubSubHubbub hub while applying the required defaults.
### Changed
- The subscribe handler now mirrors the hub's HTTP response (body/status) to the API client and falls back to the upstream status text when the hub omits a body.
- Normalized all YouTube WebSub defaults (callback URL, lease duration, verification mode) inside the handler so clients can omit them safely.
- Alert verification logging now includes the exact challenge response body so the terminal reflects what was sent back to YouTube.
