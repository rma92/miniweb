'use strict';
// service-worker.js — shell cache-first + archive offline cache.

const SHELL_CACHE   = 'mininext-shell-v2';
const ARCHIVE_CACHE = 'mininext-archives-v1';

const SHELL_FILES = [
  '/',
  '/index.html',
  '/css/app.css',
  '/js/mbpf.js',
  '/js/settings.js',
  '/js/api.js',
  '/js/renderer.js',
  '/js/app.js',
  '/manifest.json',
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(SHELL_CACHE).then(cache => cache.addAll(SHELL_FILES))
  );
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(
        keys
          .filter(k => k !== SHELL_CACHE && k !== ARCHIVE_CACHE)
          .map(k => caches.delete(k))
      )
    )
  );
  self.clients.claim();
});

self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);

  // Archive GET requests: cache-then-network (cache for offline, update when online).
  if (url.pathname.match(/^\/api\/v1\/archives\/[^/]+$/) && event.request.method === 'GET') {
    event.respondWith(archiveFetch(event.request));
    return;
  }

  // Archive list: network-only (always fresh when online, fail gracefully offline).
  if (url.pathname === '/api/v1/archives' && event.request.method === 'GET') {
    event.respondWith(
      fetch(event.request).catch(() =>
        new Response(JSON.stringify([]), { headers: { 'Content-Type': 'application/json' } })
      )
    );
    return;
  }

  // Other API calls: never cache.
  if (url.pathname.startsWith('/api/')) return;

  // App shell: cache-first.
  event.respondWith(
    caches.match(event.request).then(cached => {
      if (cached) return cached;
      return fetch(event.request).then(resp => {
        if (resp.ok && event.request.method === 'GET') {
          caches.open(SHELL_CACHE).then(cache => cache.put(event.request, resp.clone()));
        }
        return resp;
      });
    })
  );
});

// archiveFetch: serve from cache if offline, update cache when online.
async function archiveFetch(request) {
  const cached = await caches.match(request);
  try {
    const resp = await fetch(request);
    if (resp.ok) {
      const cache = await caches.open(ARCHIVE_CACHE);
      cache.put(request, resp.clone());
    }
    return resp;
  } catch (_) {
    // Offline: return cached version if available.
    if (cached) return cached;
    return new Response(
      JSON.stringify({ error: 'offline and archive not cached' }),
      { status: 503, headers: { 'Content-Type': 'application/json' } }
    );
  }
}

// Listen for messages from the app to explicitly cache or evict an archive.
self.addEventListener('message', event => {
  const { type, archiveID } = event.data || {};
  if (type === 'CACHE_ARCHIVE' && archiveID) {
    // Pre-warm: fetch and cache the archive right after saving it.
    const url = `/api/v1/archives/${archiveID}`;
    caches.open(ARCHIVE_CACHE).then(cache =>
      fetch(url, { headers: { Accept: 'application/minidom+json' } }).then(resp => {
        if (resp.ok) cache.put(url, resp);
      }).catch(() => {})
    );
  }
  if (type === 'EVICT_ARCHIVE' && archiveID) {
    caches.open(ARCHIVE_CACHE).then(cache =>
      cache.delete(`/api/v1/archives/${archiveID}`)
    );
  }
});
