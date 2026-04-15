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
    body: JSON.stringify({ version, title, body_html: bodyHTML, body_text: '', icon: '' }),
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
