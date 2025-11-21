# live-stream-alerts

A lightweight Go service that proxies YouTube WebSub subscriptions, stores streamer metadata, and exposes operational endpoints for downstream automation. The companion WebAssembly UI (alGUI) lives in its own project so UI builds and logging stay separate—you deploy the alert server and UI side by side rather than as a single binary.

## Requirements
- Go 1.21+
- (Optional) `make` for your own helper scripts

## Continuous integration
Every push/PR triggers `.github/workflows/ci.yml`, which runs `gofmt` (as a lint check), `go vet ./...`, and `go test ./...`. Run those locally before opening a PR to avoid CI failures:

```bash
gofmt -w .
go vet ./...
go test ./...
```

## Architecture
See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the layered overview (cmd ➜ app ➜ router ➜ services ➜ stores/platform clients), background workers, and testing conventions.

## Running the alert server
1. Start the HTTP server:
   ```bash
   go run ./cmd/alertserver
   ```
2. Streamer data is appended to `data/streamers.json`. Provide a different path through `CreateOptions.FilePath` if you embed the handler elsewhere. The API-only server responds to `/` with a placeholder so you remember to run the UI separately when you need the dashboard.

## Companion UI (alGUI)
Keep the UI repo (`alGUI`) checked out next to this project (for example `../alGUI`) and host/serve it independently—it's built as a standalone WASM app so you can deploy it behind any static host or local dev server. Point the UI at the alert server’s base URL (and `/api/streamers/watch` SSE endpoint) to keep your dashboards in sync while leaving alert-server logs focused solely on API/WebSub traffic.

## Configuration
The WebSub defaults can be configured via environment variables or CLI flags (flags take precedence):

| Flag | Environment variable | Description | Default |
| ---- | -------------------- | ----------- | ------- |
| `-youtube-hub-url` | `YOUTUBE_HUB_URL` | PubSubHubbub hub endpoint used for subscribe/unsubscribe flows. | `https://pubsubhubbub.appspot.com/subscribe` |
| `-youtube-callback-url` | `YOUTUBE_CALLBACK_URL` | Callback URL that the hub invokes for alert delivery. | `https://sharpen.live/alerts` |
| `-youtube-lease-seconds` | `YOUTUBE_LEASE_SECONDS` | Lease duration requested during subscribe/unsubscribe. | `864000` |
| `-youtube-default-mode` | `YOUTUBE_DEFAULT_MODE` | WebSub mode enforced when omitted (typically `subscribe`). | `subscribe` |
| `-youtube-verify-mode` | `YOUTUBE_VERIFY_MODE` | Verification strategy requested (`sync` or `async`). | `async` |

### `config.json`
The binary also reads `config.json` on startup for file-based overrides. This is the best place to pin the HTTP listener address/port alongside the YouTube defaults:

```json
{
  "admin": {
    "email": "admin@sharpen.live",
    "password": "change-me",
    "token_ttl_seconds": 86400
  },
  "server": {
    "addr": "127.0.0.1",
    "port": ":8880"
  },
  "youtube": {
    "hub_url": "https://pubsubhubbub.appspot.com/subscribe",
    "callback_url": "https://sharpen.live/alerts",
    "lease_seconds": 864000,
    "verify": "async"
  }
}
```

Omit any field to fall back to the defaults above. The legacy top-level keys (`hub_url`, `callback_url`, etc.) are still honored for backward compatibility, but nesting them under `youtube` keeps the file organized.

When `/alerts` receives a push notification, the server fetches the YouTube watch page for the referenced video, inspects its embedded metadata, and automatically updates the matching streamer record’s `status` when the notification corresponds to a live broadcast. No YouTube Data API key is required for this flow.

### YouTube lease monitor
The alert server continuously inspects `data/streamers.json` for YouTube subscriptions and automatically renews them when roughly 5% of the lease window remains. The renewal window is derived from `hubLeaseDate` (last hub confirmation) plus `leaseSeconds`, so keeping those fields current ensures subscriptions are re-upped before the hub expires them.

### Admin authentication
The admin console authenticates via `/api/admin/login`. Configure the allowed credentials in the `admin` block of `config.json`, and adjust `token_ttl_seconds` to control how long issued bearer tokens remain valid. Include the token using an `Authorization: Bearer <token>` header for any admin-only APIs.

## API reference
All HTTP routes are registered in `internal/api/v1/router.go`. Update the table below whenever an endpoint is added or altered so this README remains the single source of truth.

| Method | Path                         | Description |
| ------ | ---------------------------- | ----------- |
| GET    | `/alerts`                    | Responds to YouTube PubSubHubbub verification challenges. |
| POST   | `/api/youtube/subscribe`     | Proxies subscription requests to YouTube's hub after enforcing defaults. |
| POST   | `/api/youtube/unsubscribe`   | Issues unsubscribe calls to YouTube's hub so channels stop sending alerts. |
| POST   | `/api/youtube/channel`       | Resolves a YouTube `@handle` into its canonical channel ID. |
| GET    | `/api/streamers`             | Returns every stored streamer record. |
| GET    | `/api/streamers/watch`       | Streams server-sent events whenever `streamers.json` changes. |
| POST   | `/api/streamers`             | Queues a streamer submission for admin review (written to `data/submissions.json`). |
| PATCH  | `/api/streamers`             | Updates the alias/description/languages of an existing streamer. |
| DELETE | `/api/streamers`             | Removes a stored streamer record. |
| POST   | `/api/youtube/metadata`     | Scrapes a public URL and returns its meta description/title. |
| GET    | `/api/server/config`         | Returns the server runtime information consumed by the UI. |
| POST   | `/api/admin/login`          | Issues a bearer token for administrative API calls. |
| GET    | `/api/admin/submissions`    | Lists pending streamer submissions for review. |
| POST   | `/api/admin/submissions`    | Approves or rejects a pending submission. |
| GET    | `/api/admin/monitor/youtube`| Summarises YouTube lease status for every stored channel. |
| GET    | `/`                          | Returns placeholder text reminding you to host alGUI separately. |

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

### GET `/api/streamers/watch`
- **Purpose:** Emits Server-Sent Events whenever `data/streamers.json` changes so browser clients can reload automatically.
- **Response:** `text/event-stream` payload with an initial `ready` event followed by `change` events containing the file's modification timestamp.
- **Usage:** Connect via EventSource in the browser and call `location.reload()` when a `change` event is received.

### POST `/api/streamers`
- **Purpose:** Queues a streamer submission for admin review. Payloads still follow the schema below, but the record is written to `data/submissions.json` until an administrator approves it via `/api/admin/submissions`.
- **Request body:** Provide the streamer basics plus a single YouTube URL (optional but recommended). The values mirror the prior write-flow:
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
- **Server-managed fields:** The backend generates a submission ID and `submittedAt` timestamp. Once an admin approves the entry it is converted into a full streamer record (assigning a permanent `streamer.id`, deriving YouTube metadata, generating a hub secret, etc.).
- **Languages:** Entries must come from the supported language list (`schema/streamers.schema.json`); duplicates and blank values are rejected.
- **Validation & conflicts:** `streamer.alias` must be unique across existing streamers **and** pending submissions. Submitting a duplicate alias returns `409 Conflict`.
- **Response:** `202 Accepted` with `{ "status": "pending", "message": "Submission received..." }` when the submission is queued, or `500 Internal Server Error` if the queue write fails.

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
- **YouTube cleanup:** If the streamer has YouTube platform metadata, the server issues a PubSubHubbub `unsubscribe` before removing the record so hub callbacks stop hitting `/alerts`.
- **Responses:**
  - `200 OK` with `{ "status": "deleted", "id": "..." }` when the record is deleted.
  - `404 Not Found` if the ID does not match an existing streamer.
  - `400 Bad Request` when the ID segment is missing or the JSON body is invalid/mismatched.
  - `502 Bad Gateway` if the hub unsubscribe fails; the record remains untouched.
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

### POST `/api/admin/login`
- **Purpose:** Authenticates the admin console and returns a bearer token for subsequent admin-only API calls.
- **Request body:**
  ```json
  {
    "email": "admin@sharpen.live",
    "password": "super-secret"
  }
  ```
- **Response:**
  ```json
  {
    "token": "<bearer token>",
    "expiresAt": "2025-11-18T16:23:03Z"
  }
  ```
- **Notes:** Supply the token via `Authorization: Bearer <token>`. Tokens expire after the configured `token_ttl_seconds` duration.

### GET `/api/admin/submissions`
- **Purpose:** Returns the list of pending streamer submissions awaiting review.
- **Authentication:** Requires `Authorization: Bearer <token>` header obtained from `/api/admin/login`.
- **Response:**
  ```json
  {
    "submissions": [
      {
        "id": "sub_1731955790",
        "alias": "Knife Maker",
        "description": "Showcases livestream sharpening sessions.",
        "languages": ["English"],
        "platformUrl": "https://www.youtube.com/@knifemaker",
        "submittedAt": "2025-11-18T16:23:03Z"
      }
    ]
  }
  ```

### GET `/api/admin/monitor/youtube`
- **Purpose:** Exposes the YouTube lease monitor summary so the admin console can spot channels that are renewing or have expired leases.
- **Authentication:** Requires `Authorization: Bearer <token>` header from `/api/admin/login`.
- **Response:**
  ```json
  {
    "summary": {
      "total": 3,
      "healthy": 2,
      "renewing": 1,
      "expired": 0,
      "pending": 0
    },
    "records": [
      {
        "streamerId": "4b8e82c4a16e49e58c1ac2993e7f85e0",
        "alias": "Attorney Melanie Little",
        "channelId": "UCFSlI8Y3Zdoq5buNW_40AAA",
        "handle": "@AttorneyMelanieLittle",
        "hubUrl": "https://pubsubhubbub.appspot.com/subscribe",
        "callbackUrl": "https://sharpen.live/dev/alerts",
        "leaseSeconds": 864000,
        "leaseStart": "2025-11-18T10:07:14Z",
        "leaseExpires": "2025-11-28T10:07:14Z",
        "renewAt": "2025-11-27T14:31:14Z",
        "renewWindowSeconds": 43200,
        "status": "healthy",
        "issues": []
      }
    ]
  }
  ```
- **Statuses:** `healthy` (outside the renewal window), `renewing` (inside the window but not yet expired), `expired` (lease window elapsed), and `pending` (missing data such as a lease start or lease length). Each record’s `issues` array calls out missing/invalid fields so operators know what needs to be corrected.

### POST `/api/admin/submissions`
- **Purpose:** Approves or rejects a submission, removing it from the pending list.
- **Request body:**
  ```json
  {
    "action": "approve",
    "id": "sub_1731955790"
  }
  ```
- **Notes:** `action` can be `approve` or `reject`. The response echoes the removed submission and resulting status.

### Static asset hosting
- Requests to `/` now respond with `UI assets not configured` so deployments keep alGUI on its own host (and out of the alert server’s logs). Serve the WASM bundle from the `alGUI` project directly.

## Keeping this document current
Whenever you introduce or modify an endpoint:
1. Update `internal/api/v1/router.go` (or the relevant router) as usual.
2. Add or edit the corresponding row in the API table above.
3. Expand the detailed section for that endpoint with request/response notes.
This commitment ensures the README includes **every endpoint from now and into the future**.
