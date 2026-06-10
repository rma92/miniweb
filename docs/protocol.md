# MiniNext Protocol Reference

MiniNext exposes a REST API over HTTP(S). All request and response bodies are JSON unless noted.
Authentication is optional (controlled by `AUTH_ENABLED` env var). When enabled, include
`Authorization: Bearer <token>` on every request.

---

## Sessions

### Create Session

```
POST /api/v1/sessions
Content-Type: application/json

{
  "device_profile": "phone-modern",
  "capabilities": {
    "page_formats": ["minidom-text", "mbpf"],
    "compression": ["gzip", "brotli"],
    "image_formats": ["jpeg", "webp", "png", "gif"],
    "rendering_profiles": ["box", "flow"]
  }
}
```

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
Response `200`: `{"status":"sleeping"}`

### Resume Session

```
POST /api/v1/sessions/{session_id}/resume
```
Response `200`: `{"status":"active"}`

---

## Tabs

### Create Tab

```
POST /api/v1/sessions/{session_id}/tabs
Content-Type: application/json

{ "url": "https://example.com" }
```

Response `200`: `{"tab_id":"tab_..."}`

### Navigate

```
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/navigate
Content-Type: application/json

{ "url": "https://example.com/page" }
```

Response `200`: `{"status":"ok"}`

---

## Snapshot

Retrieves the current rendered page for a tab.

```
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/snapshot
Accept: application/minidom+json        (or application/x-mbpf)
Accept-Encoding: gzip, br
```

Query params (debug overrides):
- `format=minidom-text` or `format=mbpf`

Response `200`:
- Content-Type: `application/minidom+json` or `application/x-mbpf`
- Content-Encoding: `gzip` or `br` if negotiated
- Header `X-Snapshot-Id`: snapshot sequence number
- Body: MiniDOM Text JSON or MBPF binary (see format specs below)

---

## Interact

Dispatches a user interaction and returns an updated snapshot.

```
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/interact
Content-Type: application/json

{
  "snapshot_id": 42,
  "event": {
    "type": "click",
    "element_id": 12345
  }
}
```

Event types: `click`, `tap`, `input`, `change`, `submit`

For `input`/`change` events include `"value": "text"`.

Response `200`:
```json
{
  "ok": true,
  "snapshot_id": 43,
  "url": "https://example.com/result",
  "title": "Result page",
  "snapshot": { ... }
}
```

---

## Resources

Fetches and recompresses an image resource referenced in a snapshot.

```
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/resources/{resource_id}
```

Response `200` with `Content-Type: image/jpeg` (or png/gif/webp).
Response is cached for subsequent requests within the session.

---

## MiniDOM Text Format

MiniDOM Text is a JSON object:

```json
{
  "format": "minidom-text",
  "version": 1,
  "snapshot_id": 42,
  "url": "https://example.com/",
  "title": "Example Domain",
  "nodes": [
    {
      "id": 1,
      "type": "DOCUMENT",
      "parent_id": 0,
      "children": [2],
      "text": "",
      "layout": {"x": 0, "y": 0, "w": 390, "h": 844},
      "style": {"color": "rgb(0,0,0)", "font_size": "16px", "display": "block"}
    },
    {
      "id": 2,
      "type": "HEADING",
      "parent_id": 1,
      "text": "Example Domain",
      "layout": {"x": 10, "y": 20, "w": 370, "h": 32}
    },
    {
      "id": 3,
      "type": "LINK",
      "parent_id": 1,
      "text": "More information",
      "interaction": {
        "element_id": 1,
        "kind": "link",
        "enabled": true,
        "href": "https://www.iana.org/domains/example",
        "action_hint": "click"
      }
    }
  ],
  "resources": []
}
```

### Node Types

`DOCUMENT`, `SECTION`, `BLOCK`, `INLINE`, `TEXT`, `LINK`, `IMAGE`, `BUTTON`, `INPUT`,
`TEXTAREA`, `SELECT`, `OPTION`, `FORM`, `TABLE`, `TABLE_ROW`, `TABLE_CELL`, `LIST`,
`LIST_ITEM`, `HEADING`, `CANVAS_FALLBACK`, `UNKNOWN`

### Interaction Object

```json
{
  "element_id": 12345,
  "kind": "link|button|input|textarea|select|form|custom",
  "enabled": true,
  "readonly": false,
  "value": "current value",
  "placeholder": "hint text",
  "href": "https://... (links only)",
  "form_id": 0,
  "action_hint": "click|submit|type|change",
  "input_type": "text|password|email|...",
  "name": "field name"
}
```

---

## MBPF Binary Format

See `docs/mbpf-spec.md` for the full binary format specification.

Container layout:
```
magic:          4 bytes  "MBPF"
version:        varint   (currently 1)
flags:          varint
page_id:        varint
snapshot_id:    varint
profile_id:     varint
section_count:  varint
sections:       repeated { type_id: varint, byte_length: varint, data: bytes }
```

Sections present in Phase 1: STRING_TABLE(1), NODE_TREE(4), LAYOUT_TABLE(5),
INTERACTION_TABLE(6), RESOURCE_TABLE(7).

All integers use unsigned LEB-128 (identical to protobuf varint without zigzag encoding).

---

## Device Profiles

| Name | Width | Height | Mobile |
|------|-------|--------|--------|
| phone-small | 360 | 640 | yes |
| phone-modern | 390 | 844 | yes |
| tablet | 768 | 1024 | no |
| desktop-small | 1280 | 800 | no |
| desktop-large | 1920 | 1080 | no |

---

## Error Responses

All errors return JSON:
```json
{ "error": "description of the problem" }
```

HTTP status codes:
- `400` — bad request (missing/invalid parameters)
- `401` — authentication required or invalid token
- `403` — session belongs to a different user
- `404` — session or tab not found (or expired)
- `502` — remote browser error (navigation failure, etc.)
- `500` — internal server error
