# MiniNext Protocol Reference

MiniNext exposes a REST API over HTTP(S). All request and response bodies are JSON
unless noted. Authentication is optional (`auth.enabled` in config or `AUTH_ENABLED`
env var). When enabled, include `Authorization: Bearer <token>` on every request.

---

## Sessions

### Create Session

```
POST /api/v1/sessions
Content-Type: application/json
```

Request body:
```json
{
  "device_profile": "phone-modern",
  "capabilities": {
    "page_formats": ["minidom-text", "mbpf"],
    "compression": ["gzip", "brotli", "zstd"],
    "image_formats": ["jpeg", "webp", "png", "gif"],
    "rendering_profiles": ["box", "flow"],
    "adblock": true
  }
}
```

`device_profile` options: `phone-small`, `phone-modern`, `tablet`, `desktop-small`,
`desktop-large`, `custom`. Defaults to `phone-modern`.

`capabilities.adblock` (optional, `*bool`): `true` enables ad blocking for this
session if the server has ad blocking configured. `null`/omitted = use server default.

Response `200`:
```json
{
  "session_id": "sess_...",
  "expires_in_seconds": 600,
  "selected_profile": {
    "page_format": "minidom-text",
    "compression": "gzip",
    "image_format": "jpeg",
    "image_quality": "medium",
    "rendering_profile": "box"
  }
}
```

### Delete Session

```
DELETE /api/v1/sessions/{session_id}
```
Response `204 No Content`.

### Sleep Session

```
POST /api/v1/sessions/{session_id}/sleep
```
Frees browser memory while keeping session metadata. Tabs are restored on resume.
Response `200 {"status": "sleeping"}`.

### Resume Session

```
POST /api/v1/sessions/{session_id}/resume
```
Restores a sleeping session and navigates all tabs to their saved URLs.
Response `200 {"status": "active"}`.

### Toggle Ad Blocking

```
POST /api/v1/sessions/{session_id}/adblock
Content-Type: application/json
{"enabled": true}
```

Enables or disables ad blocking for the session immediately (no restart needed).
Response `200 {"adblock_enabled": true}`.

---

## Tabs

### Open Tab

```
POST /api/v1/sessions/{session_id}/tabs
Content-Type: application/json
{"url": "https://example.com"}
```

Response `200 {"tab_id": "tab_..."}`.

### Close Tab

```
DELETE /api/v1/sessions/{session_id}/tabs/{tab_id}
```
Response `204 No Content`.

### Navigate

```
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/navigate
Content-Type: application/json
{"url": "https://example.com"}
```

Optional field `"async": true` returns 202 immediately and navigates in the
background. Completion (or failure) is pushed to the tab's SSE stream (see
**Tab Events** below). When `async` is omitted or `false`, the request blocks
until the page is loaded.

Response `200 {"status": "ok"}` (synchronous) or `202 {"status": "navigating"}` (async).

Error responses (synchronous only):
- `504` on navigation timeout
- `502` on DNS/connection failure (body includes `"code"` field: `dns_failure`,
  `connection_refused`, `connection_timeout`, `tls_error`, `offline`, etc.)

### Tab Events (SSE)

```
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/events
```

Upgrades to a Server-Sent Events stream. The server pushes a JSON event
whenever an async navigation completes or fails:

```
data: {"type":"ready","url":"https://example.com"}

data: {"type":"error","message":"DNS lookup failed: no-such.host"}

: heartbeat
```

Event types:
- `ready` — async navigation completed; client should call `GET /snapshot`
- `error` — async navigation failed; `message` describes the failure

A comment-only heartbeat line is sent every 25 seconds to keep the connection
alive through proxies. The stream stays open until the client disconnects or
the session is deleted.

**Authentication:** `EventSource` cannot set custom headers, so when auth is
enabled pass the token as `?token=<value>` query parameter.

---

## Snapshots

### Get Snapshot

```
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/snapshot
```

Query parameters:
- `rendering` — `box` (default, full layout) or `flow` (linearised reading order)
- `format` — `minidom-text` or `mbpf` (overrides Accept header)
- `since` — snapshot ID of the client's current snapshot; server returns a delta if
  only a small fraction of nodes changed (see Delta Snapshots below)

Request headers:
- `Accept: application/x-mbpf` → binary MBPF format
- `Accept: application/minidom+json` → JSON format (default)
- `Accept-Encoding: zstd, br, gzip` → server selects best supported encoding

Response headers:
- `Content-Type: application/x-mbpf` or `application/minidom+json`
- `Content-Encoding: zstd` / `br` / `gzip` (when compressed)
- `X-Snapshot-Id: <int>` — monotonically increasing snapshot counter for this tab

#### Delta Snapshots

When `?since=<snapshot_id>` is provided and the server's cached base snapshot
matches, the server computes a node-level diff. If fewer than 60% of nodes
changed, it returns a compact delta instead of the full snapshot:

```
Content-Type: application/minidom-delta+json
```

Delta body:
```json
{
  "base_snapshot_id": 5,
  "snapshot_id": 6,
  "url": "...",
  "title": "...",
  "favicon_url": "...",
  "instructions": [
    {"op": 1, "stable_id": "a1b2c3d4", "node": { ... }},
    {"op": 2, "stable_id": "e5f6a7b8"},
    {"op": 3, "stable_id": "c9d0e1f2", "node": { ... }}
  ]
}
```

Operations: `1` = add node, `2` = remove node (by stable_id), `3` = update node.

---

## Interaction

### Interact

```
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/interact
Content-Type: application/json
```

Request body:
```json
{
  "snapshot_id": 5,
  "rendering_profile": "box",
  "event": {
    "type": "click",
    "element_id": 42
  }
}
```

Event types: `click`, `tap`, `input` (with `value`), `change` (with `value`),
`submit`, `scroll` (with `scroll_x`, `scroll_y`).

Response `200` — includes the post-interaction snapshot:
```json
{
  "ok": true,
  "snapshot_id": 6,
  "url": "https://example.com/next",
  "title": "Next Page",
  "snapshot": { ... }
}
```

---

## Resources

### Get Resource (Image)

```
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/resources/{resource_id}
```

Returns the recompressed image for a resource referenced by an IMAGE node's
`resource_id` field. Responses include `ETag` and `Cache-Control: public, max-age=3600`.
Supports `If-None-Match` for 304 responses.

---

## Archives (Offline Reading)

Archives are enabled by setting `archive.enabled: true` in the config.
Archived pages are stored as compressed MBPF (brotli) with images inlined.

### Save Archive

```
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/archive
```

Captures the current tab in flow mode with inline images and stores it for
offline reading. The oldest archive is auto-deleted when `max_per_user` is exceeded.

Response `200`:
```json
{
  "archive_id": "abc123...",
  "url": "https://example.com",
  "title": "Example Domain",
  "size": 12345
}
```

### List Archives

```
GET /api/v1/archives
```

Returns all archives for the authenticated user, newest first.

Response `200` — array of:
```json
[
  {
    "id": "abc123...",
    "url": "https://example.com",
    "title": "Example Domain",
    "favicon_url": "...",
    "size": 12345,
    "created_at": "2026-06-11T12:34:56Z"
  }
]
```

### Get Archive

```
GET /api/v1/archives/{archive_id}
```

Returns the archived snapshot. Accepts the same `Accept` header as the snapshot
endpoint. Supports `ETag` / `If-None-Match` caching.

### Delete Archive

```
DELETE /api/v1/archives/{archive_id}
```

Response `204 No Content`.

---

## Metrics

```
GET /metrics
```

Prometheus-format metrics. No authentication required.

Key metrics:
- `mininext_http_requests_total{method, path, status}`
- `mininext_http_request_duration_seconds{method, path}`
- `mininext_snapshot_bytes{format, rendering}`
- `mininext_snapshot_compressed_bytes{format, compression}`
- `mininext_active_sessions`
- `mininext_adblock_requests_blocked_total`
- `mininext_full_snapshots_sent_total`
- `mininext_delta_snapshots_sent_total`

---

## Admin API

Requires `Authorization: Bearer <admin_token>` (set via `archive.admin_token` in
config or `ADMIN_TOKEN` env var). Returns 403 if admin token is not configured.

### List All Sessions

```
GET /admin/sessions
```

Returns all active sessions across all users with tab URLs and state.

### Force Delete Session

```
DELETE /admin/sessions/{session_id}
```

Force-kills a session regardless of ownership. Response `204 No Content`.

### Config View

```
GET /admin/config
```

Returns the running configuration with sensitive fields (`auth.static_token`,
`archive.admin_token`) redacted as `[redacted]`.

### Server Status

```
GET /admin/status
```

Response `200`:
```json
{
  "uptime_seconds": 3600.5,
  "sessions": 3,
  "goroutines": 42,
  "heap_mb": 128.5
}
```

---

## MiniDOM Text Format (JSON)

Content-Type: `application/minidom+json`

```json
{
  "format": "minidom-text",
  "version": 1,
  "snapshot_id": 5,
  "url": "https://example.com",
  "title": "Example Domain",
  "favicon_url": "https://example.com/favicon.ico",
  "nodes": [ ... ],
  "resources": [ ... ]
}
```

Each node:
```json
{
  "id": 1,
  "stable_id": "a1b2c3d4",
  "type": "HEADING",
  "parent_id": 0,
  "children": [2, 3],
  "text": "Example Domain",
  "layout": {"x": 8, "y": 100, "w": 374, "h": 38},
  "style": {"font_size": "24px", "font_weight": "700"},
  "interaction": {
    "element_id": 1, "kind": "link", "enabled": true,
    "href": "https://iana.org/", "action_hint": "click"
  },
  "resource_id": "res_..."
}
```

Node types: `DOCUMENT`, `SECTION`, `BLOCK`, `INLINE`, `TEXT`, `LINK`, `IMAGE`,
`BUTTON`, `INPUT`, `TEXTAREA`, `SELECT`, `OPTION`, `FORM`, `TABLE`, `TABLE_ROW`,
`TABLE_CELL`, `LIST`, `LIST_ITEM`, `HEADING`, `CANVAS_FALLBACK`, `UNKNOWN`.

Each resource:
```json
{
  "resource_id": "res_...",
  "url": "https://example.com/img.jpg",
  "mime_type": "image/jpeg",
  "width": 640,
  "height": 480,
  "inline_data": "<base64>"
}
```

`inline_data` is present for archived snapshots; otherwise the client fetches via
the resource endpoint.

---

## MBPF Binary Format

Content-Type: `application/x-mbpf`

See `docs/mbpf-spec.md` for the full binary format specification.

Sections present in a full snapshot:
| ID | Name              | Contents                                     |
|----|-------------------|----------------------------------------------|
|  1 | STRING_TABLE      | Length-prefixed UTF-8 strings                |
|  2 | PAGE_META         | URL, title, favicon_url (string table refs)  |
|  4 | NODE_TREE         | Node records with stable_id, type, flags     |
|  5 | LAYOUT_TABLE      | Fixed-point (×10) coordinates                |
|  6 | INTERACTION       | ElementID, kind, href, value, etc.           |
|  7 | RESOURCE_TABLE    | ResourceRef entries with optional InlineData |
| 11 | DELTA_INSTRUCTIONS| (reserved for future binary delta encoding) |

Node record fields (in order):
`id`, `type_id`, `flags`, `parent_id`, `text_idx`, `stable_id_idx`,
then conditionally: `layout_idx` (if flags&1), `resource_id_idx` (if flags&4),
`interaction_idx` (if flags&2).

---

## Device Profiles

| Name           | Width | Height | Scale | Touch | UA                    |
|----------------|-------|--------|-------|-------|-----------------------|
| phone-small    | 360   | 640    | 2×    | yes   | Android Chrome        |
| phone-modern   | 390   | 844    | 3×    | yes   | iPhone Safari         |
| tablet         | 768   | 1024   | 2×    | yes   | iPad Safari           |
| desktop-small  | 1280  | 800    | 1×    | no    | Desktop Chrome        |
| desktop-large  | 1920  | 1080   | 1×    | no    | Desktop Chrome        |
| custom         | 1280  | 800    | 1×    | no    | Desktop Chrome        |

---

## Persistence Across Restarts

MiniNext supports two independent persistence mechanisms.

### Session and Tab Persistence

Set `session.persistence_db` in `config.yaml` (or `SESSION_PERSISTENCE_DB` env var) to a
bbolt file path (e.g. `sessions.db`). When enabled:

- Every `CreateSession`, `OpenTab`, `Navigate`, `CloseTab`, and `DeleteSession` call
  atomically updates the database.
- On startup, MiniNext reads all records and re-creates sessions in the browser worker,
  navigating each tab to its last known URL.
- Clients can reconnect using their saved `session_id` and `tab_id` values without
  re-authenticating or re-navigating.

If a tab's navigation fails on restore (e.g. network error), the tab is still registered
and can be navigated manually by the client.

### Cookie and localStorage Persistence

Set `browser.user_data_dir` in `config.yaml` (or `BROWSER_USER_DATA_DIR` env var) to a
directory path (e.g. `.browser_profile`). Chromium stores cookies, localStorage,
IndexedDB, and cached credentials in this directory. When the server restarts, the browser
reuses the same profile, so logins and preferences on visited sites survive restarts.

All browser workers share a single user data directory, making this most suitable for
single-user deployments. For multi-user deployments, omit this setting to use Chromium's
default in-memory profile.

---

## Sandbox and Process Isolation

### Chromium's Built-in Sandbox

By default, MiniNext launches Chromium with `--no-sandbox` for compatibility with
Docker and other unprivileged container environments. On bare-metal Linux hosts with
user-namespace support, you can disable this to let Chromium run its own highly
effective process sandbox:

```yaml
browser:
  no_sandbox: false
```

Or set `BROWSER_NO_SANDBOX=false` in the environment.

### OS-Level Sandbox Wrapper

For an additional isolation layer, configure `browser.sandbox_wrapper` with a prefix
command (firejail, bubblewrap, etc.). MiniNext generates a wrapper script at startup
and uses it as the Chromium exec path.

**firejail** (simplest):
```yaml
browser:
  sandbox_wrapper: ["firejail", "--"]
```

**bubblewrap** (more control — adjust paths for your distro):
```yaml
browser:
  sandbox_wrapper:
    - "bwrap"
    - "--ro-bind"; - "/usr"; - "/usr"
    - "--ro-bind"; - "/lib64"; - "/lib64"
    - "--dev"; - "/dev"
    - "--proc"; - "/proc"
    - "--tmpfs"; - "/tmp"
    - "--unshare-net"
    - "--"
```

The wrapper receives all Chromium flags as positional arguments via `"$@"`.

### Extra Chromium Flags

Use `browser.extra_flags` to pass arbitrary Chromium flags for hardening or tuning:

```yaml
browser:
  extra_flags:
    - "--disable-extensions"
    - "--disable-plugins"
    - "--js-flags=--max-old-space-size=256"
```
