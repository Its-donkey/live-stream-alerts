# live-stream-alerts

A lightweight Go service that proxies YouTube WebSub subscriptions, stores streamer metadata, and exposes operational endpoints for downstream automation. The companion WebAssembly UI now lives in a sibling project and consumes these APIs separately.

## Requirements
- Go 1.21+
- (Optional) `make` for your own helper scripts

## Running the alert server
1. Start the HTTP server:
   ```bash
   go run ./cmd/alertserver
   ```
2. Streamer data is appended to `data/streamers.json`. Provide a different path through `CreateOptions.FilePath` if you embed the handler elsewhere.
3. (Optional) Build and host the standalone UI from the sibling project if you need a dashboard; it talks to these APIs over HTTP and no longer ships with the server binary (this repository intentionally stays API-only).

## API reference
All HTTP routes are registered in `internal/api/v1/router.go`. Update the table below whenever an endpoint is added or altered so this README remains the single source of truth.

| Method | Path                         | Description |
| ------ | ---------------------------- | ----------- |
| GET    | `/alerts`                    | Responds to YouTube PubSubHubbub verification challenges. |
| POST   | `/api/youtube/subscribe`     | Proxies subscription requests to YouTube's hub after enforcing defaults. |
| POST   | `/api/youtube/unsubscribe`   | Issues unsubscribe calls to YouTube's hub so channels stop sending alerts. |
| POST   | `/api/youtube/channel`       | Resolves a YouTube `@handle` into its canonical channel ID. |
| GET    | `/api/streamers`             | Returns every stored streamer record. |
| POST   | `/api/streamers`             | Persists streamer metadata to `data/streamers.json`. |
| PATCH  | `/api/streamers`             | Updates the alias/description/languages of an existing streamer. |
| DELETE | `/api/streamers`             | Removes a stored streamer record. |
| POST   | `/api/youtube/metadata`     | Scrapes a public URL and returns its meta description/title. |
| GET    | `/api/server/config`         | Returns the server runtime information consumed by the UI. |

### GET `/alerts`
- **Purpose:** Handles `hub.challenge` callbacks from YouTube during WebSub verification.
- **Query parameters:** `hub.mode`, `hub.topic`, `hub.lease_seconds`, `hub.verify_token`, and **required** `hub.challenge`.
- **Response:** `200 OK` with the challenge echoed as plain text when successful; `400 Bad Request` if the challenge is missing.

### POST `/api/youtube/subscribe`
- **Purpose:** Submits an application/x-www-form-urlencoded request to YouTube's hub (`https://pubsubhubbub.appspot.com/subscribe`).
- **Request body:** JSON matching `internal/platforms/youtube/subscriptions.YouTubeRequest`:
  - `topic` (required): full feed URL to subscribe to.
  - `verify` (optional): `"sync"` or `"async"`; defaults to `"async"`.
  - `verifyToken`, `secret`, `leaseSeconds` (optional) pass-through fields.
- **Server-managed defaults:**
  - `callback` is pinned to `https://sharpen.live/alert`.
  - `mode` is forced to `"subscribe"`.
  - `leaseSeconds` falls back to `864000` (10 days) when omitted.
- **Response:** Mirrors the upstream hub's status code, headers, and body. When the hub omits a body, the handler writes the upstream status text.

### POST `/api/youtube/unsubscribe`
- **Purpose:** Sends an unsubscribe request to YouTube's hub so the callback stops receiving push notifications for the provided topic.
- **Request body:** Matches `POST /api/youtube/subscribe`; only `topic` is required and defaults mirror the subscribe handler.
- **Response:** Mirrors the upstream hub's status code, headers, and body. When the hub omits a body, the handler writes the upstream status text.

### POST `/api/youtube/channel`
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

### GET `/api/streamers`
- **Purpose:** Lists every persisted streamer record so the UI or tooling can inspect the latest state.
- **Response:** `200 OK` with `{ "streamers": [ ...records... ] }`.
- **Notes:** Records mirror the schema in `schema/streamers.schema.json`, including platform metadata and server-managed timestamps.

### POST `/api/streamers`
- **Purpose:** Appends a streamer record to `data/streamers.json` using the schema in `schema/streamers.schema.json`.
- **Request body:** Provide the streamer basics plus a single YouTube URL. The server derives the streamer ID, resolves the channel handle/ID, stores those fields, and subscribes to hub notifications:
  ```json
  {
    "streamer": {
      "alias": "SharpenDev",
      "description": "Tantalum chef knife maker focusing on live sharpening Q&A.",
      "languages": ["English"]
    },
    "platforms": {
      "url": "https://www.youtube.com/@SharpenDev"
    }
  }
  ```
- **Server-managed fields:** `streamer.id` is derived from the alias by stripping non-alphanumeric characters. IDs, timestamps, and platform metadata are set by the server. Once the record is created it is immediately enriched with the channel handle and ID taken from the supplied URL (or by resolving the @handle), and the backend generates a fresh hub secret that it later uses for WebSub HMAC validation.
- **YouTube subscriptions:** After the record is stored the backend resolves any missing channel metadata, saves it back to `data/streamers.json`, and issues the PubSubHubbub subscription. Failures are logged but do not block the initial `201 Created` response.
- **Languages:** When provided, entries must come from the supported language list (see `schema/streamers.schema.json`); duplicates and blank values are rejected.
- **Validation:** `streamer.alias` must be non-empty and unique once cleaned (submitting a duplicate alias returns `409 Conflict`). The `platforms.url` value must be a valid YouTube channel URL when provided.
- **Response:** `201 Created` with the stored record echoed back as JSON, or `500 Internal Server Error` if the file append fails.

### DELETE `/api/streamers`
- **Purpose:** Removes a streamer record (including its platform metadata) from `data/streamers.json`.
- **Request:** Provide the streamer ID in the body:
  ```json
  {
    "streamer": {
      "id": "SharpenDev"
    }
  }
  ```
- **Notes:** The path no longer requires the ID segment; only the JSON body must include `streamer.id` (case-insensitive match).
- **Responses:**
  - `200 OK` with `{ "status": "deleted", "id": "..." }` when the record is deleted.
  - `404 Not Found` if the ID does not match an existing streamer.
  - `400 Bad Request` when the ID segment is missing or the JSON body is invalid/mismatched.
  - `500 Internal Server Error` for unexpected persistence failures (also logged server-side).
- **Handler coverage:** The same `/api/streamers` handler powers GET, POST, PATCH, and DELETE, so clients can reuse the base path and expect the `Allow: GET, POST, PATCH, DELETE` header on unsupported verbs.

### PATCH `/api/streamers`
- **Purpose:** Partially updates an existing streamer identified by `streamer.id`, allowing operators to refresh the alias, description, or languages without recreating the record.
- **Request body:** Provide the ID plus any mutable fields:
  ```json
  {
    "streamer": {
      "id": "SharpenDev",
      "alias": "Sharpen Dev",
      "description": "Sharper knives every Wednesday.",
      "languages": ["English", "Japanese"]
    }
  }
  ```
- **Validation:** `streamer.id` is required. Alias cannot be blank when supplied. Languages reuse the same allow-list/duplicate trimming as the create endpoint; invalid values return `400 Bad Request`. At least one mutable field must be present.
- **Response:** `200 OK` with the updated streamer record echoed back. `404 Not Found` is returned if the ID does not exist.

### GET `/api/server/config`
- **Purpose:** Exposes runtime metadata consumed by companion tooling (including the standalone UI).
- **Response:**
  ```json
  {
    "name": "live-stream-alerts",
    "addr": "127.0.0.1",
    "port": ":8880",
    "readTimeout": "10s"
  }
  ```

### POST `/api/youtube/metadata`
- **Purpose:** Returns the `<meta name="description">` (or OpenGraph description) plus related channel metadata for a supplied public URL so tooling can pre-fill streamer descriptions, display names, and YouTube identifiers.
- **Request body:**
  ```json
  {
    "url": "https://www.youtube.com/@example"
  }
  ```
- **Response:**
  ```json
  {
    "description": "Channel summary pulled from the destination site.",
    "title": "Example Channel",
    "handle": "@example",
    "channelId": "UCabc123"
  }
  ```
- **Notes:** Only `http`/`https` URLs are allowed. A `502` is returned if scraping fails or the metadata cannot be extracted.

### Static asset hosting
- Requests to `/` now respond with `200 OK` and `"UI assets not configured"`. The standalone UI is built, versioned, and hosted from the sibling project instead of bundling inside this repository.

## Keeping this document current
Whenever you introduce or modify an endpoint:
1. Update `internal/api/v1/router.go` (or the relevant router) as usual.
2. Add or edit the corresponding row in the API table above.
3. Expand the detailed section for that endpoint with request/response notes.
This commitment ensures the README includes **every endpoint from now and into the future**.
