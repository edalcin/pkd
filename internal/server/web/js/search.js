/**
 * search.js — Universal substring search
 * Debounced 150ms, ESC to clear, click to open result.
 */
import { apiFetch } from './app.js';

export function initSearch(onSelect) {
  const input = document.getElementById('search-input');
  if (!input) return;

  let timer = null;
  let resultsEl = null;

  function getOrCreateResults() {
    if (!resultsEl) {
      resultsEl = document.createElement('div');
      resultsEl.id = 'search-results';
      resultsEl.style.cssText = 'position:absolute;top:calc(var(--topbar-height) + 2px);left:50%;transform:translateX(-50%);width:min(480px,90vw);background:var(--bg-card);border:1px solid var(--border);border-radius:8px;box-shadow:var(--shadow);z-index:200;max-height:60vh;overflow-y:auto';
      document.body.appendChild(resultsEl);
    }
    return resultsEl;
  }

  function clear() {
    if (resultsEl) { resultsEl.remove(); resultsEl = null; }
  }

  input.addEventListener('keydown', e => {
    if (e.key === 'Escape') { input.value = ''; clear(); }
  });

  input.addEventListener('input', () => {
    clearTimeout(timer);
    const q = input.value.trim();
    if (!q) { clear(); return; }
    timer = setTimeout(async () => {
      const res = await apiFetch('/api/search?q=' + encodeURIComponent(q));
      if (!res.ok) return;
      const hits = await res.json();
      const container = getOrCreateResults();
      container.innerHTML = '';
      if (!hits || !hits.length) {
        container.innerHTML = '<div style="padding:.75rem;color:var(--text-muted);font-size:.875rem">No results</div>';
        return;
      }
      for (const hit of hits) {
        const item = document.createElement('div');
        item.style.cssText = 'padding:.625rem .75rem;cursor:pointer;border-bottom:1px solid var(--border)';
        item.innerHTML = `<div style="font-weight:500;font-size:.875rem">${hit.title}</div><div style="font-size:.8rem;color:var(--text-muted);margin-top:.125rem">${hit.snippet || ''}</div>`;
        item.addEventListener('mouseenter', () => item.style.background = 'var(--bg-hover)');
        item.addEventListener('mouseleave', () => item.style.background = '');
        item.addEventListener('click', () => {
          clear();
          input.value = '';
          onSelect(hit.id);
        });
        container.appendChild(item);
      }
    }, 150);
  });

  document.addEventListener('click', e => {
    if (!e.target.closest('#search-input') && !e.target.closest('#search-results')) clear();
  });
}
