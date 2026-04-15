/**
 * app.js — PKD SPA bootstrap
 *
 * Responsibilities:
 * - Read the CSRF token from the pkd_csrf cookie and attach it to every mutating fetch.
 * - Mount the tree and editor on load.
 * - Handle theme toggle (light/dark) persisted in localStorage.
 * - Handle logout.
 * - Register the service worker.
 */

import { initTree, loadTree } from './tree.js';
import { initEditor, openDocument } from './editor.js';

/* ── CSRF token ──────────────────────────────────────────── */
export function csrfToken() {
  const match = document.cookie.split(';').find(c => c.trim().startsWith('pkd_csrf='));
  return match ? match.split('=')[1].trim() : '';
}

export async function apiFetch(path, options = {}) {
  const method = (options.method || 'GET').toUpperCase();
  const mutating = !['GET', 'HEAD'].includes(method);
  const headers = { ...(options.headers || {}) };
  if (mutating) {
    headers['X-CSRF-Token'] = csrfToken();
    if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }
  }
  const res = await fetch(path, { ...options, headers });
  // Detect offline-mode response from the service worker
  if (res.headers.get('x-pkd-offline') === 'read-only') {
    document.dispatchEvent(new CustomEvent('pkd:offline'));
  }
  return res;
}

/* ── Theme ───────────────────────────────────────────────── */
function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem('pkd-theme', theme);
}

function toggleTheme() {
  const current = document.documentElement.dataset.theme || 'light';
  applyTheme(current === 'light' ? 'dark' : 'light');
}

/* ── Logout ──────────────────────────────────────────────── */
async function logout() {
  await apiFetch('/api/logout', { method: 'POST' });
  window.location.replace('/login');
}

/* ── Offline banner ──────────────────────────────────────── */
function showOfflineBanner() {
  let banner = document.getElementById('offline-banner');
  if (!banner) {
    banner = document.createElement('div');
    banner.id = 'offline-banner';
    banner.textContent = 'offline — read only';
    document.body.prepend(banner);
  }
}

/* ── Init ────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', async () => {
  // Restore theme
  applyTheme(localStorage.getItem('pkd-theme') || 'light');

  // Wire controls
  document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
  document.getElementById('logout-btn')?.addEventListener('click', logout);
  document.getElementById('menu-toggle')?.addEventListener('click', () => {
    document.getElementById('sidebar')?.classList.toggle('collapsed');
  });

  // Offline detection
  document.addEventListener('pkd:offline', showOfflineBanner);

  // Init tree and editor
  initTree({ onSelect: openDocument });
  initEditor();
  await loadTree();

  // Register service worker
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(console.warn);
  }
});
