// PKD Service Worker — Svelte 5 app (003-pkm-refactor)
// Strategy:
//   index.html (/) → network-first (picks up UI updates; cache fallback for offline)
//   manifest.webmanifest + Vite assets → cache-first (content-hash filenames)
//   GET /api/documents/:id → stale-while-revalidate (LRU of last 100 docs)
//   Mutating requests (POST/PUT/DELETE/PATCH) → network-only; offline → 503
//   PWA share_target (POST /api/capture) → passes through to network (auth via cookie)
const SHELL_CACHE = 'pkd-shell-v6';
const DOC_CACHE   = 'pkd-docs-v5';
const DOC_MAX     = 100;

// App shell: index.html + manifest.
// Vite asset files use content-hash names so they're cached at runtime.
const SHELL_ASSETS = [
  '/',
  '/manifest.webmanifest',
];

// ─── Install ───────────────────────────────────────────────────────────────
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(SHELL_CACHE)
      .then(cache => cache.addAll(SHELL_ASSETS))
      .then(() => self.skipWaiting())
  );
});

// ─── Activate: purge stale caches ─────────────────────────────────────────
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(
        keys
          .filter(k => k !== SHELL_CACHE && k !== DOC_CACHE)
          .map(k => caches.delete(k))
      ))
      .then(() => self.clients.claim())
  );
});

// ─── Fetch ────────────────────────────────────────────────────────────────
self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  // Only intercept same-origin requests
  if (url.origin !== self.location.origin) return;

  const method = request.method;
  const isMutation = !['GET', 'HEAD'].includes(method);
  const isDocAPI   = /^\/api\/documents\/\d+$/.test(url.pathname) && method === 'GET';
  const isIndex    = url.pathname === '/';
  const isShell    = url.pathname === '/manifest.webmanifest';
  const isAsset    = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/icons/');

  if (isMutation) {
    // All writes (including POST /api/capture from share_target) go straight to network.
    // When offline, return a structured 503 so the app can show an error.
    event.respondWith(networkOrOffline(request));
  } else if (isDocAPI) {
    // Document GET: stale-while-revalidate for offline read-only access
    event.respondWith(staleWhileRevalidate(request));
  } else if (isIndex) {
    // index.html: network-first so UI updates are picked up without CTRL+F5.
    // Falls back to cache only when offline.
    event.respondWith(networkFirst(request));
  } else if (isShell || isAsset) {
    // manifest + Vite-hashed assets: cache-first (content-hash = implicit versioning)
    event.respondWith(cacheFirst(request));
  }
  // Everything else → browser default (network)
});

// ─── Strategies ──────────────────────────────────────────────────────────

async function networkOrOffline(request) {
  try {
    return await fetch(request);
  } catch {
    return new Response(JSON.stringify({ error: 'offline', mode: 'read-only' }), {
      status: 503,
      headers: {
        'Content-Type': 'application/json',
        'x-pkd-offline': '1',
      },
    });
  }
}

async function networkFirst(request) {
  const cache = await caches.open(SHELL_CACHE);
  try {
    const res = await fetch(request);
    if (res.ok) await cache.put(request, res.clone());
    return res;
  } catch {
    return (await cache.match(request)) || new Response('', { status: 503 });
  }
}

async function staleWhileRevalidate(request) {
  const cache = await caches.open(DOC_CACHE);
  const cached = await cache.match(request);

  const networkFetch = fetch(request).then(async res => {
    if (res.ok) {
      await evictLRU(cache, DOC_MAX - 1);
      await cache.put(request, res.clone());
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
  if (res.ok) await cache.put(request, res.clone());
  return res;
}

async function evictLRU(cache, max) {
  const keys = await cache.keys();
  if (keys.length >= max) {
    await cache.delete(keys[0]);
  }
}
