/**
 * editor.js — PKD rich document editor (CKEditor 5)
 *
 * Features:
 * - Loads ClassicEditor from /vendor/ckeditor5/ckeditor.js
 * - Auto-save with 2-second idle debounce
 * - Optimistic-concurrency save: sends current version, handles 409
 * - Conflict dialog with overwrite / reload choices (FR-010a)
 * - Inline image upload via SimpleUploadAdapter → POST /api/documents/{id}/attachments
 * - Offline detection: disables editor + shows banner (clarification Q1)
 */

import { apiFetch } from './app.js';

// Available icon keys (must match internal/security/icons.go whitelist)
const ICONS = ['document','folder','star','bookmark','tag','image','link','code','book','idea','note','calendar','task','archive','heart','flag'];
let currentIcon = '';

let editor = null;
let currentDocId = null;
let currentVersion = null;
let saveTimer = null;
let offline = false;

/* ── Init ────────────────────────────────────────────────── */
export function initEditor() {
  document.addEventListener('pkd:offline', () => {
    offline = true;
    if (editor) {
      editor.enableReadOnlyMode('offline');
      const banner = document.getElementById('offline-banner');
      if (banner) banner.classList.remove('hidden');
    }
  });

  // Conflict dialog buttons
  document.getElementById('conflict-overwrite')?.addEventListener('click', forceOverwrite);
  document.getElementById('conflict-reload')?.addEventListener('click', reloadStored);

  document.addEventListener('pkd:docDeselected', () => {
    currentDocId = null;
    currentVersion = null;
    if (editor) editor.setData('');
    const titleEl = document.getElementById('doc-title');
    if (titleEl) titleEl.value = '';
  });
}

/* ── Open a document ─────────────────────────────────────── */
export async function openDocument(id) {
  clearSaveTimer();
  const res = await apiFetch(`/api/documents/${id}`);
  if (!res.ok) return;
  const doc = await res.json();

  currentDocId = doc.id;
  currentVersion = doc.version;
  currentIcon = doc.icon || '';

  const titleEl = document.getElementById('doc-title');
  if (titleEl) {
    titleEl.value = doc.title;
    titleEl.onchange = () => scheduleSave();
  }

  await mountEditor(doc.body_html);
}

/* ── CKEditor mount ──────────────────────────────────────── */
async function mountEditor(initialData) {
  const container = document.getElementById('editor');
  if (!container) return;

  if (editor) {
    editor.destroy();
    editor = null;
  }

  // ClassicEditor is loaded as a global by the vendor bundle
  const CKEditor = window.ClassicEditor;
  if (!CKEditor) {
    // Lazy-load the vendor script on first use
    await loadScript('/vendor/ckeditor5/ckeditor.js');
  }

  editor = await window.ClassicEditor.create(container, {
    initialData: initialData || '',
    simpleUpload: {
      uploadUrl: () => `/api/documents/${currentDocId}/attachments`,
      withCredentials: true,
      headers: { 'X-CSRF-Token': getCsrfToken() },
    },
  });

  editor.model.document.on('change:data', () => {
    if (!offline) scheduleSave();
  });

  if (offline) {
    editor.enableReadOnlyMode('offline');
  }
}

async function loadScript(src) {
  return new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.src = src;
    s.onload = resolve;
    s.onerror = reject;
    document.head.appendChild(s);
  });
}

/* ── Auto-save ───────────────────────────────────────────── */
function scheduleSave() {
  clearSaveTimer();
  saveTimer = setTimeout(save, 2000);
}

function clearSaveTimer() {
  if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; }
}

async function save(forceVersion = null) {
  if (!currentDocId || offline) return;
  const titleEl = document.getElementById('doc-title');
  const title = titleEl?.value.trim() || 'Untitled';
  const bodyHTML = editor?.getData() || '';
  // body_text is derived server-side via sanitizer
  const version = forceVersion ?? currentVersion;

  const res = await apiFetch(`/api/documents/${currentDocId}`, {
    method: 'PUT',
    body: JSON.stringify({ version, title, body_html: bodyHTML, body_text: '', icon: currentIcon }),
  });

  if (res.ok) {
    const doc = await res.json();
    currentVersion = doc.version;
    return;
  }

  if (res.status === 409) {
    const conflict = await res.json();
    showConflictDialog(conflict);
  }
}

/* ── Conflict dialog ─────────────────────────────────────── */
let pendingConflict = null;

function showConflictDialog(conflict) {
  pendingConflict = conflict;
  document.getElementById('conflict-dialog')?.classList.remove('hidden');
}

function hideConflictDialog() {
  document.getElementById('conflict-dialog')?.classList.add('hidden');
  pendingConflict = null;
}

async function forceOverwrite() {
  if (!pendingConflict) return;
  hideConflictDialog();
  await save(pendingConflict.stored_version);
}

async function reloadStored() {
  if (!pendingConflict) return;
  const stored = pendingConflict.stored;
  hideConflictDialog();
  currentVersion = stored.version;
  const titleEl = document.getElementById('doc-title');
  if (titleEl) titleEl.value = stored.title;
  if (editor) editor.setData(stored.body_html || '');
}

/* ── Helpers ─────────────────────────────────────────────── */
function getCsrfToken() {
  const m = document.cookie.split(';').find(c => c.trim().startsWith('pkd_csrf='));
  return m ? m.split('=')[1].trim() : '';
}

/* ── Icon picker (T101) ──────────────────────────────────────────────────── */
export function openIconPicker() {
  document.getElementById('icon-picker-modal')?.remove();
  const modal = document.createElement('div');
  modal.id = 'icon-picker-modal';
  modal.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.5);display:flex;align-items:center;justify-content:center;z-index:300';

  const card = document.createElement('div');
  card.style.cssText = 'background:var(--bg-card);border-radius:8px;padding:1rem;width:min(360px,90vw);box-shadow:0 8px 32px rgba(0,0,0,.2)';

  const title = document.createElement('div');
  title.textContent = 'Choose Icon';
  title.style.cssText = 'font-weight:600;margin-bottom:.75rem;font-size:.875rem';
  card.appendChild(title);

  const grid = document.createElement('div');
  grid.style.cssText = 'display:grid;grid-template-columns:repeat(6,1fr);gap:.5rem';

  // "None" option
  const none = document.createElement('button');
  none.textContent = '✕';
  none.title = 'No icon';
  none.style.cssText = 'padding:.5rem;border:1px solid var(--border);border-radius:6px;cursor:pointer;background:var(--bg);color:var(--text)';
  none.onclick = () => { selectIcon(''); modal.remove(); };
  grid.appendChild(none);

  for (const key of ICONS) {
    const btn = document.createElement('button');
    btn.title = key;
    btn.style.cssText = 'padding:.5rem;border:1px solid var(--border);border-radius:6px;cursor:pointer;background:' + (currentIcon === key ? 'var(--bg-active)' : 'var(--bg)') + ';color:var(--text);font-size:.75rem';
    btn.textContent = key.charAt(0).toUpperCase();
    // Load the actual SVG icon
    fetch('/icons/' + key + '.svg').then(r => r.text()).then(svg => { btn.innerHTML = svg; });
    btn.onclick = () => { selectIcon(key); modal.remove(); };
    grid.appendChild(btn);
  }

  card.appendChild(grid);
  modal.appendChild(card);
  modal.addEventListener('click', e => { if (e.target === modal) modal.remove(); });
  document.body.appendChild(modal);
}

function selectIcon(key) {
  currentIcon = key;
  const iconBtn = document.getElementById('doc-icon-btn');
  if (iconBtn) iconBtn.textContent = key || '📄';
  scheduleSave();
}
