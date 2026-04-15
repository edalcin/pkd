/**
 * app.js — PKD SPA bootstrap
 */

import { initTree, loadTree } from './tree.js';
import { initEditor, openDocument } from './editor.js';

/* ── CSRF token ──────────────────────────────────────────── */
export function csrfToken() {
  const m = document.cookie.split(';').find(c => c.trim().startsWith('pkd_csrf='));
  return m ? m.slice(m.indexOf('=') + 1).trim() : '';
}

export async function apiFetch(path, options = {}) {
  const method  = (options.method || 'GET').toUpperCase();
  const mutating = !['GET', 'HEAD'].includes(method);
  const headers  = { ...(options.headers || {}) };

  if (mutating) {
    headers['X-CSRF-Token'] = csrfToken();
    if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }
  }

  const res = await fetch(path, { ...options, headers });

  if (res.headers.get('x-pkd-offline') === 'read-only') {
    document.dispatchEvent(new CustomEvent('pkd:offline'));
  }
  return res;
}

/* ── Tema ────────────────────────────────────────────────── */
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

/* ── Novo documento raiz ─────────────────────────────────── */
async function newRootDocument() {
  const res = await apiFetch('/api/documents', {
    method: 'POST',
    body: JSON.stringify({ title: 'Novo documento' }),
  });
  if (!res.ok) return;
  const doc = await res.json();
  await loadTree();
  openDocument(doc.id);
}

/* ── Banner offline ──────────────────────────────────────── */
function showOfflineBanner() {
  let banner = document.getElementById('offline-banner');
  if (!banner) {
    banner = document.createElement('div');
    banner.id = 'offline-banner';
    banner.textContent = 'offline — somente leitura';
    document.body.prepend(banner);
  }
}

/* ── Init ────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', async () => {
  // Restaurar tema salvo
  applyTheme(localStorage.getItem('pkd-theme') || 'light');

  // Controles da topbar
  document.getElementById('theme-toggle')?.addEventListener('click', toggleTheme);
  document.getElementById('logout-btn')?.addEventListener('click', logout);
  document.getElementById('new-doc-btn')?.addEventListener('click', newRootDocument);
  document.getElementById('menu-toggle')?.addEventListener('click', () => {
    document.getElementById('sidebar')?.classList.toggle('collapsed');
  });

  // Detecção de offline
  document.addEventListener('pkd:offline', showOfflineBanner);

  // Inicializar árvore e editor
  initTree({ onSelect: openDocument });
  initEditor();

  // Carregar árvore de documentos
  await loadTree();

  // Registrar Service Worker (PWA)
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  }
});
