// PKD Service Worker — hand-written, no Workbox
// Strategy:
//   App shell assets → cache-first with network fallback (pre-cached on install)
//   GET /api/documents/:id → stale-while-revalidate (LRU of last 100)
//   Mutating requests (POST/PUT/DELETE/PATCH) → network-first; offline → 503 + x-pkd-offline header
const SHELL_CACHE = 'pkd-shell-v3';
const DOC_CACHE   = 'pkd-docs-v3';
const DOC_MAX     = 100;

const SHELL_ASSETS = [
  '/',
  '/login',
  '/css/app.css',
  '/js/app.js',
  '/js/tree.js',
  '/js/editor.js',
  '/js/search.js',
  '/js/tags.js',
  '/js/calendar.js',
  '/js/admin.js',
  '/js/share-view.js',
  '/vendor/ckeditor5/ckeditor.js',
  '/manifest.webmanifest',
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(SHELL_CACHE).then(cache => cache.addAll(SHELL_ASSETS))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(keys
        .filter(k => k !== SHELL_CACHE && k !== DOC_CACHE)
        .map(k => caches.delete(k))
      )
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  // Only handle same-origin requests
  if (url.origin !== self.location.origin) return;

  const isMutation = !['GET', 'HEAD'].includes(request.method);
  const isDocAPI   = /^\/api\/documents\/\d+$/.test(url.pathname) && request.method === 'GET';
  const isShell    = SHELL_ASSETS.includes(url.pathname);

  if (isMutation) {
    event.respondWith(networkOrOffline(request));
  } else if (isDocAPI) {
    event.respondWith(staleWhileRevalidate(request));
  } else if (isShell) {
    event.respondWith(cacheFirst(request));
  }
  // All other requests fall through to the browser default (network).
});

async function networkOrOffline(request) {
  try {
    return await fetch(request);
  } catch {
    return new Response(JSON.stringify({ error: 'offline — read only' }), {
      status: 503,
      headers: {
        'Content-Type': 'application/json',
        'x-pkd-offline': 'read-only',
      },
    });
  }
}

async function staleWhileRevalidate(request) {
  const cache = await caches.open(DOC_CACHE);
  const cached = await cache.match(request);
  const networkFetch = fetch(request).then(async res => {
    if (res.ok) {
      await evictLRU(cache, DOC_MAX - 1);
      cache.put(request, res.clone());
    }
    return res;
  }).catch(() => null);

  return cached || (await networkFetch) || new Response('', { status: 503 });
}

async function cacheFirst(request) {
  const cache = await caches.open(SHELL_CACHE);
  const cached = await cache.match(request);
  if (cached) return cached;
  const res = await fetch(request);
  if (res.ok) cache.put(request, res.clone());
  return res;
}

async function evictLRU(cache, max) {
  const keys = await cache.keys();
  if (keys.length >= max) {
    await cache.delete(keys[0]);
  }
}
