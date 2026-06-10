# Specification: Modern Remote-Rendered Compressed Browser Successor to Opera Mini

## 1. Working Name

**Project Codename:** MiniNext
**Product Type:** Remote-rendered, bandwidth-efficient, cross-platform browser system
**Primary Inspiration:** Opera Mini

## 2. Summary

MiniNext is a modern successor to Opera Mini’s core model: a lightweight client communicates with a server-side browser engine that loads, executes, renders, simplifies, serializes, and compresses web pages.

The server runs an existing modern browser engine, preferably headless Chrome/Chromium or Firefox. The remote browser executes website JavaScript, handles modern web compatibility, maintains cookies and sessions, renders the page, and exports a simplified rendered representation to the client.

The client does **not** need to execute JavaScript from visited pages. The client may itself be implemented in JavaScript, such as the initial HTML5/PWA client, but remote page JavaScript runs only in the server-side browser.

MiniNext supports multiple users and can be run as a hosted service. Authentication is optional by deployment mode, but the architecture must support authenticated multi-user operation from the start.

The system also supports optional future archival and offline-reading features by exporting compressed rendered page snapshots.

## 3. Goals

The system must:

1. Provide modern web compatibility through an existing remote browser engine.
2. Execute website JavaScript on the remote server-side browser.
3. Send the rendered result to a lightweight client.
4. Avoid requiring the client to execute target-site JavaScript.
5. Minimize bandwidth through structural page serialization, image recompression, and transport compression.
6. Keep the client simple enough to implement in HTML5, JavaScript, C, C++, Java ME-like environments, or other limited runtimes.
7. Support multiple users.
8. Support optional authentication.
9. Support hosted-service deployment as well as self-hosted deployment.
10. Preserve remote website sessions across requests.
11. Allow browser sessions to sleep and later resume state.
12. Support multiple tabs within a single remote browser session.
13. Allow interaction with links, buttons, forms, inputs, selects, scroll containers, and other ordinary page elements.
14. Support configurable viewport size, device profile, user agent, image quality, image format, compression, and server policy.
15. Offer optional server-side ad blocking, such as uBlock Origin or equivalent filtering in the remote browser.
16. Expose server functionality through a REST API over HTTPS by default, with optional HTTP for trusted/private deployments.
17. Provide both a readable text representation and an efficient binary representation of the rendered page format.
18. Support optional export of compressed rendered pages for archive and offline reading use cases.

## 4. Non-Goals for Initial Version

The first version does not need to support:

1. Video playback.
2. Real-time animation fidelity.
3. WebGL streaming.
4. WebRTC.
5. Browser extension management UI.
6. Full remote desktop streaming.
7. Pixel-perfect reproduction of every page.
8. Client-side execution of target-site JavaScript.
9. Perfect accessibility tree reconstruction, although the protocol should leave room for it.

## 5. Core Architecture

MiniNext consists of four major components:

1. **Client Application**

   * Presents a browser-like user interface.
   * Sends navigation and interaction requests to the server.
   * Receives compact page snapshots or page deltas.
   * Decodes MiniDOM Text or MBPF binary payloads.
   * Renders simplified but interactive page output.
   * Maintains local UI state such as current tab, scroll position, settings, offline reading list, and connection status.

2. **Server API**

   * Written in Go.
   * Exposes a REST API.
   * Manages users, sessions, tabs, authentication, authorization, browser workers, compression settings, and security policy.
   * Owns the canonical browser state.
   * Serializes rendered pages into MiniDOM Text or MBPF.

3. **Remote Browser Worker**

   * Runs an existing modern browser.
   * Preferred initial engines: headless Chrome/Chromium or Firefox.
   * Executes website JavaScript server-side.
   * Handles cookies, sessions, storage, layout, form state, and DOM.
   * Produces rendered page snapshots, interactive element maps, text, images, style summaries, and layout metadata.

4. **Archive/Offline Store**

   * Optional later component.
   * Stores compressed rendered page snapshots.
   * Can support offline reading lists and archive-like permanent page captures.
   * Must be designed so it can be disabled for privacy-sensitive deployments.

## 6. Multi-User and Deployment Model

MiniNext must support multiple users from the beginning.

Supported deployment modes:

### 6.1 Personal Self-Hosted Mode

A single user runs the server for personal use.

Characteristics:

1. Authentication may be disabled or minimal.
2. HTTP may be allowed for trusted LAN deployments.
3. Browser isolation can be lighter.
4. Persistent browser profile may be convenient.
5. Admin and user may be the same person.

### 6.2 Multi-User Self-Hosted Mode

An organization or group runs the server for multiple users.

Characteristics:

1. Authentication should be enabled.
2. Users must have isolated sessions.
3. Per-user quotas should be available.
4. Admin configuration controls session limits, storage, and retention.
5. Browser profiles must not leak across users.

### 6.3 Hosted Service Mode

MiniNext can be operated as a public or private hosted service.

Characteristics:

1. Authentication is required.
2. Strong browser worker isolation is required.
3. Session data retention must be explicit and configurable.
4. User data must be protected at rest where feasible.
5. Per-user resource quotas are required.
6. Abuse prevention, rate limiting, and logging are required.
7. The service must clearly disclose that remote browsing is visible to the server operator.

## 7. User Model

A user may be:

1. Anonymous.
2. Authenticated.
3. Admin.
4. Service operator.

A user owns:

1. Remote browser sessions.
2. Tabs.
3. Saved settings.
4. Optional persistent browser profiles.
5. Optional archived rendered pages.
6. Optional offline reading list.

Recommended user fields:

```json
{
  "user_id": "usr_...",
  "display_name": "Richard",
  "auth_mode": "local|oauth|token|anonymous",
  "default_device_profile": "phone-modern",
  "default_image_quality": "medium",
  "default_image_format": "webp",
  "default_compression": "gzip",
  "max_sessions": 5,
  "max_tabs_per_session": 10,
  "archive_enabled": true
}
```

## 8. Remote Browser Session Model

### 8.1 Session

A session represents one remote browsing environment. It includes:

1. User ID or anonymous token.
2. Browser profile directory or equivalent storage.
3. Cookies.
4. Local storage.
5. Session storage where feasible.
6. IndexedDB where feasible.
7. Cache metadata, subject to policy.
8. Open tabs/windows.
9. Active viewport profile.
10. Server-side settings.
11. Expiration policy.
12. Sleep/resume state.

### 8.2 Session Retention

Website sessions must be retained on the remote browser side. This allows the user to stay logged into sites across requests and short periods of inactivity.

Default session expiration:

```text
Default idle expiration: 10 minutes
```

This must be configurable server-side.

Recommended configuration:

```yaml
session:
  idle_timeout_seconds: 600
  max_lifetime_seconds: 86400
  allow_persistent_profiles: true
  anonymous_session_timeout_seconds: 600
  authenticated_session_timeout_seconds: 86400
```

### 8.3 Sleeping Sessions

A session may be put to sleep when inactive.

A sleeping session should retain:

1. Cookies.
2. Storage.
3. Tab list.
4. Last known URLs.
5. Navigation history where possible.
6. Form state where possible.
7. Last serialized page snapshot.
8. Scroll positions.
9. Viewport profile.

A sleeping session may release:

1. Live browser process.
2. Renderer process memory.
3. In-memory JavaScript heap.
4. Temporary network connections.

Resume behavior:

1. If the browser process is still alive, resume directly.
2. If the browser process was terminated, recreate it using saved profile state.
3. Reload tabs as needed.
4. Restore URLs, viewport, and saved tab metadata.
5. Return a cached snapshot immediately where possible, then update after reload.

## 9. Remote Browser Engine

MiniNext should use an existing browser engine rather than implementing a browser.

Preferred initial choices:

1. Headless Chrome/Chromium.
2. Headless Firefox.

The server architecture must abstract the browser engine so the project can switch or support multiple backends.

Required remote browser behavior:

1. Load modern websites.
2. Execute website JavaScript.
3. Process CSS and layout.
4. Handle cookies and storage.
5. Support forms and navigation.
6. Expose DOM, layout boxes, screenshots, and network events to the server.
7. Allow viewport and user-agent configuration.
8. Allow optional content blocking.

The browser automation layer may use:

1. Chrome DevTools Protocol.
2. Playwright-style browser automation.
3. Firefox remote debugging protocols.
4. A custom abstraction over one or more of the above.

## 10. Device Profile and Viewport Configuration

Each session or tab may specify a device profile.

A device profile includes:

```json
{
  "profile_name": "phone-small",
  "viewport_width": 360,
  "viewport_height": 640,
  "device_scale_factor": 2,
  "user_agent": "configured user agent string",
  "touch": true,
  "mobile": true,
  "preferred_color_scheme": "light",
  "accept_language": "en-US,en;q=0.9"
}
```

Required profiles:

1. `phone-small`
2. `phone-modern`
3. `tablet`
4. `desktop-small`
5. `desktop-large`
6. `custom`

The server must allow administrators to define custom profiles.

## 11. Client Requirements

### 11.1 Initial HTML5/PWA Client

The first client must be:

1. HTML5-based.
2. Installable as a PWA.
3. Usable from iPhone Safari.
4. Usable from Android Chrome.
5. Usable from desktop Chrome and Edge.
6. Able to store itself locally where browser platform support allows.
7. Able to communicate with the server over HTTPS.
8. Able to use HTTP if explicitly configured.
9. Written so that the protocol decoder and renderer are easy to understand and port.

The HTML5 client may be written in JavaScript. This does not conflict with the rule that visited-page JavaScript executes only on the remote browser.

### 11.2 Client Browser UI

The client should include:

1. Address/search bar.
2. Back.
3. Forward.
4. Reload.
5. Stop.
6. Tabs.
7. Page viewport.
8. Scrollable content.
9. Connection/session status.
10. Settings screen.
11. Optional offline reading list screen.
12. Optional archive/save-page action.

### 11.3 Client Settings Screen

The client must include a settings screen.

Settings should include at minimum:

1. Server endpoint URL.
2. HTTPS or HTTP mode, subject to server support.
3. Authentication token or login state.
4. Default image quality:

   * High
   * Medium
   * Low
5. Preferred image format:

   * PNG
   * GIF
   * JPEG
   * WebP
6. Compression preference where exposed:

   * Server default
   * Gzip
   * Brotli
   * None
7. Device profile:

   * Phone
   * Tablet
   * Desktop
   * Custom, if supported
8. Offline reading settings:

   * Download over Wi-Fi only, optional
   * Include images, optional
   * Maximum stored pages, optional
9. Clear local cache.
10. Clear remote session.
11. Log out.

Example client settings object:

```json
{
  "endpoint": "https://mininext.example.com",
  "allow_http": false,
  "image_quality": "medium",
  "image_format": "webp",
  "compression": "server-default",
  "device_profile": "phone-modern",
  "offline_reading": {
    "enabled": true,
    "include_images": true,
    "max_pages": 100
  }
}
```

### 11.4 Low-End Client Target

The protocol should be suitable for clients with performance comparable to:

1. Hypothetical J2ME app on Motorola Razr V3-class hardware.

   * Around 400 MHz CPU.
   * Around 64 MB RAM.
   * Small screen.
   * Slow network.

2. Native C/C++ app on an old PC.

   * Reasonable target: 486-class machine.
   * Around 32 MB RAM.
   * Basic graphics environment.

This means:

1. Decoder must be simple.
2. Avoid mandatory CSS layout engine.
3. Avoid mandatory JavaScript execution.
4. Avoid mandatory complex image codecs for legacy profiles.
5. Allow server-side text wrapping and line breaking.
6. Allow flow-mode payloads.
7. Allow page chunking.
8. Allow text MiniDOM for development and debugging.
9. Allow binary MBPF for production efficiency.

## 12. Page Extraction Model

The remote browser renders the page normally, including executing website JavaScript. The server then extracts a compact representation suitable for thin clients.

Extraction modes:

1. **Structured Mode**

   * Preferred default.
   * Extracts page structure, text, links, forms, images, layout boxes, simplified styles, and interaction metadata.
   * Client renders simplified HTML-like content locally.

2. **Text/Flow Mode**

   * Lower-fidelity mode.
   * Linearizes page content.
   * Useful for legacy clients, debugging, and offline reading.
   * Can resemble a reader-mode or simplified Opera Mini-like rendering.

3. **Graphical Fallback Mode**

   * Inspired by WRP-style image-map browsing.
   * Server renders viewport or page regions as compressed images.
   * Server sends clickable/tappable regions.
   * Used for complex pages, canvas-heavy pages, unsupported layouts, or debugging.
   * Optional after MVP.

## 13. MiniDOM

MiniDOM is the normalized post-render representation of a page.

MiniDOM is not raw HTML. It is a compact representation of the rendered page after the remote browser has executed JavaScript, applied CSS, performed layout, and resolved dynamic DOM state.

MiniDOM should have two encodings:

1. **MiniDOM Text**

   * Human-readable.
   * Easier for client developers.
   * Useful for debugging, tests, development tools, and simple prototype clients.
   * May use JSON, line-oriented records, or a compact textual syntax.

2. **MBPF**

   * Mini Binary Page Format.
   * Efficient binary encoding of the same conceptual model.
   * Intended for production and low-bandwidth clients.

Both encodings must represent the same core data model.

## 14. MiniDOM Core Data Model

Each node may include:

1. Node ID.
2. Node type.
3. Tag or semantic role.
4. Text content.
5. Safe attributes.
6. Computed style subset.
7. Layout box.
8. Child nodes.
9. Interaction metadata.
10. Resource references.
11. Accessibility hints, optional.

Required node types:

```text
DOCUMENT
SECTION
BLOCK
INLINE
TEXT
LINK
IMAGE
BUTTON
INPUT
TEXTAREA
SELECT
OPTION
FORM
TABLE
TABLE_ROW
TABLE_CELL
LIST
LIST_ITEM
HEADING
CANVAS_FALLBACK
UNKNOWN
```

The client should not need to implement the full HTML specification.

## 15. MiniDOM Text Encoding

MiniDOM Text should be used for:

1. Early development.
2. Debugging.
3. Test fixtures.
4. Protocol inspection.
5. Simple experimental clients.
6. Archive inspection.

Possible format options:

1. JSON.
2. JSON Lines.
3. S-expressions.
4. A compact custom line format.

Recommendation for Phase 0:

Use JSON or JSON Lines first.

Example:

```json
{
  "format": "minidom-text",
  "version": 1,
  "snapshot_id": 42,
  "url": "https://example.com",
  "title": "Example",
  "nodes": [
    {
      "id": 1,
      "type": "DOCUMENT",
      "children": [2]
    },
    {
      "id": 2,
      "type": "HEADING",
      "text": "Example Domain",
      "layout": [0, 0, 320, 40],
      "style": 3
    },
    {
      "id": 3,
      "type": "LINK",
      "text": "More information",
      "interaction": {
        "element_id": 100,
        "kind": "link",
        "href": "https://www.iana.org/domains/example"
      }
    }
  ]
}
```

MiniDOM Text should not be the final low-bandwidth protocol, but it should remain supported as a development and diagnostics format.

## 16. MBPF: Mini Binary Page Format

MBPF is the production binary encoding of MiniDOM.

MBPF must be:

1. Compact.
2. Streamable.
3. Versioned.
4. Easy to decode on low-power devices.
5. Independent of Go, JavaScript, or any one language.
6. Friendly to dictionary compression.
7. Suitable for whole-page snapshots and incremental deltas.

### 16.1 MBPF Container

Each MBPF payload begins with:

```text
magic:       4 bytes  "MBPF"
version:     varint
flags:       varint
page_id:     varint
snapshot_id: varint
profile_id:  varint
section_count: varint
sections:    repeated section
checksum:    optional
```

### 16.2 MBPF Sections

Recommended sections:

```text
STRING_TABLE
TAG_TABLE
STYLE_TABLE
NODE_TREE
LAYOUT_TABLE
INTERACTION_TABLE
RESOURCE_TABLE
IMAGE_TABLE
FORM_STATE
SCROLL_STATE
DELTA_INSTRUCTIONS
ARCHIVE_METADATA optional
DEBUG_INFO optional
```

### 16.3 Node Encoding

Each node record:

```text
node_id: varint
node_type: varint
flags: varint
parent_id_or_depth: varint
text_id: optional varint
style_id: optional varint
layout_id: optional varint
interaction_id: optional varint
resource_id: optional varint
child_count_or_end_marker: optional
```

Depth-first stream encoding is preferred for low-memory clients.

### 16.4 Delta Encoding

After the initial full snapshot, the server may send deltas.

Delta operations:

```text
INSERT_NODE
REMOVE_NODE
REPLACE_NODE
UPDATE_TEXT
UPDATE_STYLE
UPDATE_LAYOUT
UPDATE_IMAGE
UPDATE_FORM_VALUE
UPDATE_SCROLL
UPDATE_TITLE
UPDATE_URL
SET_LOADING_STATE
```

The client must be allowed to request a full snapshot if it cannot apply a delta.

## 17. Layout Model

MiniDOM should support two rendering profiles:

### 17.1 Box Mode

Uses server-computed coordinates.

Each visible node may include:

```text
x
y
width
height
display
visibility
z_order
scroll_container_id
```

Advantages:

1. Higher fidelity.
2. Less client-side layout logic.
3. Better for modern clients.

Disadvantages:

1. More payload data.
2. Less adaptable to different client screens unless generated per viewport.

### 17.2 Flow Mode

Uses simplified document flow.

Advantages:

1. Easier for low-end clients.
2. Lower bandwidth.
3. Better for text-oriented browsing.
4. Useful for offline reading.

Disadvantages:

1. Lower fidelity.
2. More like reader mode than full page rendering.

## 18. Interaction Model

Interactive nodes include:

```json
{
  "element_id": 12345,
  "kind": "link|button|input|textarea|select|checkbox|radio|form|custom",
  "enabled": true,
  "readonly": false,
  "value": "current value if safe",
  "placeholder": "placeholder text",
  "href": "visible or resolved URL for links",
  "form_id": 77,
  "action_hint": "click|submit|focus|type|change"
}
```

The client must use `element_id` for interactions, not raw DOM selectors.

Supported event types:

```text
click
tap
focus
blur
input
change
submit
keydown
scroll
select
back
forward
reload
stop
```

## 19. Compression

Compression must be configurable.

Initial compression options:

```text
none
gzip
brotli
xz/lzma
zstd optional
```

Minimum required:

1. `none`
2. `gzip`

Recommended soon after:

1. `brotli`
2. `zstd`

Future/experimental:

1. `xz`
2. `lzma`
3. `7z-compatible container`

Compression should be negotiated per client.

Compression profiles:

1. **fast**

   * Prioritizes CPU speed.
   * Uses gzip or zstd-fast.

2. **balanced**

   * Uses brotli or zstd moderate settings.
   * Good default for modern devices.

3. **maximum**

   * Uses brotli high settings, xz/lzma, or other high-ratio codecs.
   * Intended for very slow or expensive networks.

4. **legacy**

   * Uses gzip only.
   * Avoids complex client decoders.

## 20. Image Processing

The client settings screen must allow the user to select image quality and preferred image format.

Required image quality options:

```text
high
medium
low
```

Required image format options:

```text
PNG
GIF
JPEG
WebP
```

Recommended future option:

```text
AVIF
```

Server image policy example:

```yaml
images:
  enabled: true
  default_quality: medium
  default_format: webp
  allowed_formats:
    - png
    - gif
    - jpeg
    - webp
  max_width: 800
  max_height: 1200
  strip_metadata: true
```

Image delivery options:

1. Inline in the MiniDOM/MBPF payload for tiny images.
2. Separate resource endpoint.
3. Lazy-loaded when the client scrolls.
4. Included in offline/archive bundles when requested.

## 21. Optional Archive Feature

A later feature should allow users or services to export and store compressed rendered pages.

This can support:

1. Archive-like permanent captures.
2. Offline reading.
3. Research snapshots.
4. Sharing simplified rendered pages.
5. Low-bandwidth saved-page transfer.

The archive should store the rendered page representation, not necessarily the original page source.

An archive entry may include:

1. Original URL.
2. Final URL after redirects.
3. Capture timestamp.
4. Page title.
5. MiniDOM Text or MBPF payload.
6. Compression algorithm.
7. Image resources.
8. Image quality and format settings.
9. Viewport profile.
10. User agent used.
11. Optional screenshot or thumbnail.
12. Optional content hash.
13. Optional expiration or retention policy.

Example archive metadata:

```json
{
  "archive_id": "arc_...",
  "owner_user_id": "usr_...",
  "original_url": "https://example.com",
  "final_url": "https://example.com",
  "title": "Example Domain",
  "captured_at": "2026-06-10T12:00:00Z",
  "format": "mbpf",
  "format_version": 1,
  "compression": "brotli",
  "image_format": "webp",
  "image_quality": "medium",
  "device_profile": "phone-modern",
  "public": false
}
```

Important privacy rule:

Archive storage must be opt-in or explicitly enabled by deployment policy. Some pages may contain private data.

## 22. Offline Reading List

A related later client feature should allow users to save pages for offline reading.

The offline reading list should:

1. Request an archive/offline snapshot from the server.
2. Store the compressed rendered page locally where the client platform allows.
3. Store associated image resources if enabled.
4. Allow reading without a server connection.
5. Mark whether a saved page is complete or partial.
6. Allow deletion from local storage.
7. Optionally sync saved-page metadata with the server.

Offline reading should support:

1. Save current page.
2. Save without images.
3. Save with low/medium/high images.
4. Save for flow-mode reading.
5. View saved pages.
6. Delete saved pages.
7. Clear all offline data.

Offline reading does not imply that interactive page functionality works offline. Saved pages are primarily static rendered snapshots.

## 23. REST API Draft

### 23.1 Create Session

```http
POST /api/v1/sessions
```

Request:

```json
{
  "device_profile": "phone-modern",
  "capabilities": {
    "page_formats": ["minidom-text", "mbpf"],
    "compression": ["gzip", "brotli"],
    "image_formats": ["webp", "jpeg", "png", "gif"],
    "rendering_profiles": ["box", "flow"]
  }
}
```

Response:

```json
{
  "session_id": "sess_...",
  "expires_in_seconds": 600,
  "selected_profile": {
    "page_format": "mbpf",
    "compression": "gzip",
    "image_format": "webp",
    "image_quality": "medium",
    "rendering_profile": "box"
  }
}
```

### 23.2 Create Tab

```http
POST /api/v1/sessions/{session_id}/tabs
```

Request:

```json
{
  "url": "https://example.com"
}
```

### 23.3 Navigate

```http
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/navigate
```

Request:

```json
{
  "url": "https://example.com"
}
```

### 23.4 Get Snapshot

```http
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/snapshot
Accept: application/x-mbpf
Accept-Encoding: gzip, br
```

Alternative for development:

```http
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/snapshot
Accept: application/minidom+json
```

### 23.5 Interact

```http
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/interact
```

Request:

```json
{
  "snapshot_id": 42,
  "event": {
    "type": "click",
    "element_id": 12345
  }
}
```

### 23.6 Resource Fetch

```http
GET /api/v1/sessions/{session_id}/tabs/{tab_id}/resources/{resource_id}
```

### 23.7 Sleep Session

```http
POST /api/v1/sessions/{session_id}/sleep
```

### 23.8 Resume Session

```http
POST /api/v1/sessions/{session_id}/resume
```

### 23.9 Delete Session

```http
DELETE /api/v1/sessions/{session_id}
```

### 23.10 Save Archive Snapshot

Optional later endpoint:

```http
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/archive
```

Request:

```json
{
  "format": "mbpf",
  "compression": "brotli",
  "include_images": true,
  "image_quality": "medium",
  "rendering_profile": "flow",
  "visibility": "private"
}
```

Response:

```json
{
  "archive_id": "arc_...",
  "status": "saved"
}
```

### 23.11 Get Archive Snapshot

```http
GET /api/v1/archives/{archive_id}
```

### 23.12 Offline Reading Bundle

Optional later endpoint:

```http
POST /api/v1/sessions/{session_id}/tabs/{tab_id}/offline-bundle
```

Returns a compressed bundle containing MiniDOM/MBPF payload plus selected resources.

## 24. Server Requirements

The server must be written in Go.

Supported platforms:

1. Linux.
2. BSD-like UNIX systems where practical.
3. macOS for development.
4. Windows where practical.

The Go server should include:

1. HTTP API router.
2. Authentication middleware.
3. User manager.
4. Session manager.
5. Tab manager.
6. Browser worker manager.
7. Browser automation adapter.
8. Page extraction pipeline.
9. MiniDOM Text encoder.
10. MBPF encoder.
11. Compression pipeline.
12. Image recompression pipeline.
13. Archive/offline bundle service, optional later.
14. Cache manager.
15. Policy engine.
16. Logging and metrics.
17. Admin configuration loader.

## 25. Browser Worker Adapter

Required interface:

```go
type BrowserWorker interface {
    CreateSession(profile DeviceProfile) (SessionHandle, error)
    OpenTab(session SessionHandle, url string) (TabHandle, error)
    Navigate(tab TabHandle, url string) error
    Interact(tab TabHandle, event InteractionEvent) error
    Snapshot(tab TabHandle, options SnapshotOptions) (*PageSnapshot, error)
    CloseTab(tab TabHandle) error
    SleepSession(session SessionHandle) error
    ResumeSession(session SessionHandle) error
    DestroySession(session SessionHandle) error
}
```

The initial implementation should strongly prefer headless Chromium or Firefox through an automation protocol.

## 26. Server-Side Ad Blocking and Filtering

Server-side ad blocking is optional and not required for day one.

Potential approaches:

1. Run uBlock Origin or compatible extension in the remote browser.
2. Apply network-level filtering using filter lists.
3. Apply DNS-level blocking.
4. Apply resource-level blocking in the browser automation layer.
5. Provide per-user or per-server policy.

Configuration:

```yaml
content_filtering:
  enabled: false
  mode: "browser_extension"
  lists:
    - "easylist"
    - "easyprivacy"
  allow_user_toggle: true
```

## 27. Security and Privacy

### 27.1 Transport

1. HTTPS by default.
2. HTTP only when explicitly enabled.
3. Token-based API authentication.
4. Secure cookies for hosted service mode.
5. CSRF protection for browser clients where applicable.

### 27.2 User Isolation

For multi-user and hosted modes:

1. User sessions must be isolated.
2. Browser profiles must not be shared across users.
3. Per-user storage must be separated.
4. Per-user quotas must be enforceable.
5. Browser workers should be sandboxed.
6. Hosted deployments should consider containerized browser workers.

### 27.3 Server Trust

The server can see:

1. User browsing activity.
2. Rendered page content.
3. Cookies and session data in the remote browser.
4. Form input sent to remote pages.
5. Archived snapshots if enabled.

Therefore, the product must clearly indicate that this is remote browsing and that the server operator is trusted.

### 27.4 Archive Privacy

Archive and offline features may capture sensitive rendered content.

Requirements:

1. Archive must be opt-in or policy-controlled.
2. Private archives must be default.
3. Users must be able to delete archives.
4. Admins must configure retention.
5. Sensitive form values should not be logged.
6. Archive sharing must require explicit action.

## 28. Administration

Server configuration should support:

1. Authentication mode.
2. User registration policy.
3. Session timeout.
4. Maximum users.
5. Maximum sessions per user.
6. Maximum tabs per session.
7. Browser engine path.
8. Browser worker pool size.
9. Allowed compression algorithms.
10. Image quality defaults.
11. Allowed image formats.
12. Ad blocking policy.
13. HTTP enable/disable.
14. TLS settings.
15. Logging level.
16. Data retention.
17. Archive enable/disable.
18. Offline bundle enable/disable.
19. Per-user quotas.

Example:

```yaml
server:
  listen_addr: "0.0.0.0:8443"
  https_enabled: true
  http_enabled: false

auth:
  enabled: true
  mode: "local"

browser:
  engine: "chromium"
  worker_pool_min: 1
  worker_pool_max: 8
  headless: true

session:
  idle_timeout_seconds: 600
  max_tabs_per_session: 10
  persistent_profiles: true

users:
  max_sessions_per_user: 5
  max_storage_mb_per_user: 1024

encoding:
  default_page_format: "mbpf"
  allow_minidom_text: true
  default_compression: "gzip"
  allowed_compression:
    - "none"
    - "gzip"
    - "brotli"

images:
  default_format: "webp"
  default_quality: "medium"
  allowed_formats:
    - "png"
    - "gif"
    - "jpeg"
    - "webp"

archive:
  enabled: false
  default_visibility: "private"
  max_archives_per_user: 1000

content_filtering:
  enabled: false
```

## 29. Development Phases

### Phase 0: Research Prototype

Goals:

1. Run headless Chromium or Firefox from Go.
2. Load a URL.
3. Execute page JavaScript in the remote browser.
4. Extract rendered DOM text, links, images, forms, and bounding boxes.
5. Return MiniDOM Text as JSON.
6. Display it in a basic HTML5 client.
7. Support clicking links and submitting simple forms.

### Phase 1: Minimum Viable Product

Goals:

1. REST API.
2. Multi-user-capable architecture.
3. Optional authentication.
4. Session creation.
5. Multiple tabs.
6. Navigation.
7. Remote JavaScript execution.
8. Structured MiniDOM snapshot.
9. MiniDOM Text support.
10. Basic MBPF encoder/decoder.
11. Gzip compression.
12. Image recompression to selected format.
13. HTML5/PWA client.
14. Client settings screen.
15. Click/input/form interaction.
16. 10-minute idle timeout.
17. Basic session sleep/resume.

### Phase 2: Opera Mini-Like Usability

Goals:

1. Better tabs UI.
2. Back/forward/reload.
3. Better layout fidelity.
4. Flow and box rendering profiles.
5. Brotli support.
6. Better image policy.
7. Low-bandwidth mode.
8. Server configuration file.
9. Persistent authenticated user sessions.
10. Better error handling.

### Phase 3: Advanced Compression and Legacy Clients

Goals:

1. Delta snapshots.
2. Zstd or xz/lzma support.
3. Ultra-legacy rendering profile.
4. C/C++ reference decoder.
5. J2ME-like reference decoder or emulator target.
6. Page chunking.
7. Dictionary compression experiments.
8. More aggressive structural tokenization.

### Phase 4: Optional Server-Side Filtering

Goals:

1. Server-side ad blocking.
2. Per-site toggle.
3. Admin policy.
4. Filter list updates.
5. Compatibility reporting.

### Phase 5: Archive and Offline Reading

Goals:

1. Export compressed rendered pages.
2. Store private archives.
3. Create offline reading bundles.
4. Add client offline reading list.
5. Support local cached reading in the PWA.
6. Add archive retention and deletion.
7. Optionally support public archive sharing.

### Phase 6: Hardening

Goals:

1. Container isolation.
2. Security review.
3. Data retention controls.
4. Resource quotas.
5. Hosted-service readiness.
6. Metrics and admin UI.
7. Packaged releases for Linux, BSD, macOS, and Windows where practical.

## 30. MVP Acceptance Criteria

The MVP is acceptable when:

1. A user can open the HTML5 client.
2. The client can configure the server endpoint.
3. The client can configure image quality.
4. The client can choose PNG, GIF, JPEG, or WebP if the server supports them.
5. The client can create or authenticate into a remote session.
6. The user can navigate to a URL.
7. The remote browser loads the modern page.
8. The remote browser executes website JavaScript.
9. The server extracts the rendered result as MiniDOM.
10. The server can return MiniDOM Text for debugging/development.
11. The server can return MBPF or a transitional binary format for production testing.
12. The server compresses payloads with gzip.
13. The client renders a scrollable page.
14. The user can click links.
15. The user can fill and submit a basic form.
16. Website cookies/session state persist for at least the configured idle period.
17. A session can sleep and resume.
18. The user can open at least two tabs.
19. The server is written in Go.
20. The server runs at least on Linux.
21. The architecture supports multiple users.
22. Authentication can be enabled by deployment configuration.
23. Configuration supports a 10-minute idle timeout.
24. The protocol is documented enough for a second client implementation.

## 31. Final Product Principle

MiniNext should not attempt to rebuild a full browser on the client. The server runs a real browser, executes JavaScript, handles compatibility, and produces a simplified rendered representation. The client stays small, portable, low-bandwidth, and easy to reimplement.

The product should support both modern convenience and extreme efficiency: a PWA client for today’s devices, a binary protocol for constrained clients, and a text MiniDOM format that makes development and debugging practical.

