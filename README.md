# live-stream-alerts

A lightweight Go service that proxies YouTube WebSub subscriptions, stores streamer metadata, and serves the alGUI WebAssembly frontend alongside operational endpoints.

## Requirements
- Go 1.21+
- (Optional) `make` for your own helper scripts

## Running the alert server
1. Build the alGUI assets (served statically by the server):
   ```bash
   cd web/algui
   GOOS=js GOARCH=wasm go build -o main.wasm
   ```
2. Start the HTTP server:
   ```bash
   go run ./cmd/alertserver
   ```
   The binary serves static assets from `web/algui` by default. Set `ALGUI_STATIC_DIR` to override the asset directory.
3. Streamer data is appended to `data/streamers.json`. Provide a different path through `CreateOptions.FilePath` if you embed the handler elsewhere.

## API reference
All HTTP routes are registered in `internal/http/v1/router.go`. Update the table below whenever an endpoint is added or altered so this README remains the single source of truth.

| Method | Path                         | Description |
| ------ | ---------------------------- | ----------- |
| GET    | `/alerts`                    | Responds to YouTube PubSubHubbub verification challenges. |
| POST   | `/api/v1/youtube/subscribe`  | Proxies subscription requests to YouTube's hub after enforcing defaults. |
| POST   | `/api/v1/youtube/channel`    | Resolves a YouTube `@handle` into its canonical channel ID. |
| POST   | `/api/v1/streamers`          | Persists streamer metadata to `data/streamers.json`. |
| GET    | `/api/v1/server/config`      | Returns the server runtime information consumed by the UI. |

### GET `/alerts`
- **Purpose:** Handles `hub.challenge` callbacks from YouTube during WebSub verification.
- **Query parameters:** `hub.mode`, `hub.topic`, `hub.lease_seconds`, `hub.verify_token`, and **required** `hub.challenge`.
- **Response:** `200 OK` with the challenge echoed as plain text when successful; `400 Bad Request` if the challenge is missing.

### POST `/api/v1/youtube/subscribe`
- **Purpose:** Submits an application/x-www-form-urlencoded request to YouTube's hub (`https://pubsubhubbub.appspot.com/subscribe`).
- **Request body:** JSON matching `internal/platforms/youtube/client.YouTubeRequest`:
  - `topic` (required): full feed URL to subscribe to.
  - `verify` (optional): `"sync"` or `"async"`; defaults to `"async"`.
  - `verifyToken`, `secret`, `leaseSeconds` (optional) pass-through fields.
- **Server-managed defaults:**
  - `callback` is pinned to `https://sharpen.live/alert`.
  - `mode` is forced to `"subscribe"`.
  - `leaseSeconds` falls back to `864000` (10 days) when omitted.
- **Response:** Mirrors the upstream hub's status code, headers, and body. When the hub omits a body, the handler writes the upstream status text.

### POST `/api/v1/youtube/channel`
- **Purpose:** Converts a YouTube `@handle` into a canonical `UC...` channel ID by calling YouTube Data APIs via `ResolveChannelID`.
- **Request body:**
  ```json
  {
    "handle": "@example"
  }
  ```
- **Responses:**
  - `200 OK` with `{ "handle": "@example", "channelId": "UC..." }` when successful.
  - `400 Bad Request` if `handle` is missing.
  - `502 Bad Gateway` if channel resolution fails.

### POST `/api/v1/streamers`
- **Purpose:** Appends a streamer record to `data/streamers.json` using the schema in `schema/streamers.schema.json`.
- **Request body:** JSON that includes a `streamer` object (alias, first/last name, email, optional location) plus per-platform configuration:
  ```json
  {
    "streamer": {
      "alias": "SharpenDev",
      "firstName": "Jane",
      "lastName": "Doe",
      "email": "jane@example.com"
    },
    "platforms": {
      "youtube": {
        "handle": "@SharpenDev",
        "channelId": "UCabc123",
        "hubSecret": "..."
      }
    }
  }
  ```
- **Server-managed fields:** Any incoming `streamer.id`, `createdAt`, or `updatedAt` values are ignored; IDs and timestamps are injected when the record is stored.
- **Validation:** `streamer.firstName`, `streamer.lastName`, and `streamer.email` must be non-empty. When the YouTube block is present, `platforms.youtube.handle` is also required.
- **Response:** `201 Created` with the stored record echoed back as JSON, or `500 Internal Server Error` if the file append fails.

### GET `/api/v1/server/config`
- **Purpose:** Exposes runtime metadata consumed by the alGUI frontend.
- **Response:**
  ```json
  {
    "name": "alGUI",
    "addr": "127.0.0.1",
    "port": ":8880",
    "readTimeout": "10s"
  }
  ```

### Static asset hosting
- Requests to `/` fall back to the WebAssembly UI served from `web/algui`. When the assets are missing, the server responds with `200 OK` and the message `"alGUI assets not configured"`.

## Keeping this document current
Whenever you introduce or modify an endpoint:
1. Update `internal/http/v1/router.go` (or the relevant router) as usual.
2. Add or edit the corresponding row in the API table above.
3. Expand the detailed section for that endpoint with request/response notes.
This commitment ensures the README includes **every endpoint from now and into the future**.
