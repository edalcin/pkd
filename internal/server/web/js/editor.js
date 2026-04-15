/**
 * editor.js — PKD rich document editor (CKEditor 5)
 *
 * Requer que window.ClassicEditor já esteja definido (carregado estaticamente
 * via <script src="/vendor/ckeditor5/ckeditor.js"> no index.html).
 */

import { apiFetch } from './app.js';

const ICONS = ['document','folder','star','bookmark','tag','image','link',
               'code','book','idea','note','calendar','task','archive','heart','flag'];

let editor        = null;
let currentDocId  = null;
let currentVersion = null;
let currentIcon   = '';
let saveTimer     = null;
let offline       = false;

/* ── Init ─────────────────────────────────────────────────── */
export function initEditor() {
  document.addEventListener('pkd:offline', () => {
    offline = true;
    if (editor) editor.enableReadOnlyMode('offline');
    document.getElementById('offline-banner')?.classList.remove('hidden');
  });

  document.getElementById('conflict-overwrite')?.addEventListener('click', forceOverwrite);
  document.getElementById('conflict-reload')?.addEventListener('click', reloadStored);
  document.getElementById('doc-icon-btn')?.addEventListener('click', openIconPicker);

  document.addEventListener('pkd:docDeselected', () => {
    currentDocId = null;
    currentVersion = null;
    if (editor) editor.setData('');
    const t = document.getElementById('doc-title');
    if (t) t.value = '';
  });
}

/* ── Abrir documento ──────────────────────────────────────── */
export async function openDocument(id) {
  clearSaveTimer();
  const res = await apiFetch(`/api/documents/${id}`);
  if (!res.ok) return;
  const doc = await res.json();

  currentDocId  = doc.id;
  currentVersion = doc.version;
  currentIcon   = doc.icon || '';

  const titleEl = document.getElementById('doc-title');
  if (titleEl) {
    titleEl.value = doc.title;
    titleEl.onchange = () => scheduleSave();
    titleEl.oninput  = () => scheduleSave();
  }

  const iconBtn = document.getElementById('doc-icon-btn');
  if (iconBtn) iconBtn.textContent = currentIcon || '📄';

  await mountEditor(doc.body_html);
}

/* ── Montar CKEditor ─────────────────────────────────────── */
async function mountEditor(initialData) {
  const container   = document.getElementById('editor');
  const loadingEl   = document.getElementById('editor-loading');
  const errorEl     = document.getElementById('editor-error');

  if (!container) return;

  // Limpar estado anterior
  if (editor) {
    try { await editor.destroy(); } catch (_) {}
    editor = null;
  }
  if (errorEl) { errorEl.textContent = ''; errorEl.classList.add('hidden'); }

  // Verificar se CKEditor está disponível (deve ter sido carregado estaticamente)
  if (typeof window.ClassicEditor === 'undefined') {
    showEditorError('CKEditor não está disponível. Recarregue a página.');
    return;
  }

  if (loadingEl) loadingEl.classList.remove('hidden');

  try {
    editor = await window.ClassicEditor.create(container, {
      initialData: initialData || '',
      simpleUpload: {
        // uploadUrl como string — avaliada no momento da montagem (currentDocId já definido)
        uploadUrl: `/api/documents/${currentDocId}/attachments`,
        withCredentials: true,
        headers: { 'X-CSRF-Token': getCsrfToken() },
      },
      // Desabilitar plugins que causam problemas sem configuração adicional
      removePlugins: ['MediaEmbed'],
    });

    editor.model.document.on('change:data', () => {
      if (!offline) scheduleSave();
    });

    if (offline) editor.enableReadOnlyMode('offline');

  } catch (err) {
    console.error('CKEditor mount error:', err);
    showEditorError(`Erro ao carregar editor: ${err.message}`);
  } finally {
    if (loadingEl) loadingEl.classList.add('hidden');
  }
}

function showEditorError(msg) {
  const el = document.getElementById('editor-error');
  if (el) { el.textContent = msg; el.classList.remove('hidden'); }
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
  if (!currentDocId || offline || !editor) return;
  const titleEl = document.getElementById('doc-title');
  const title   = titleEl?.value.trim() || 'Sem título';
  const bodyHTML = editor.getData() || '';
  const version  = forceVersion ?? currentVersion;

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

/* ── Diálogo de conflito ─────────────────────────────────── */
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
  return m ? m.slice(m.indexOf('=') + 1).trim() : '';
}

/* ── Seletor de ícone ────────────────────────────────────── */
export function openIconPicker() {
  document.getElementById('icon-picker-modal')?.remove();
  const modal = document.createElement('div');
  modal.id = 'icon-picker-modal';
  modal.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.5);display:flex;align-items:center;justify-content:center;z-index:300';

  const card = document.createElement('div');
  card.style.cssText = 'background:var(--bg-card);border-radius:8px;padding:1rem;width:min(360px,90vw);box-shadow:0 8px 32px rgba(0,0,0,.2)';

  const heading = document.createElement('div');
  heading.textContent = 'Escolher ícone';
  heading.style.cssText = 'font-weight:600;margin-bottom:.75rem;font-size:.875rem;color:var(--text)';
  card.appendChild(heading);

  const grid = document.createElement('div');
  grid.style.cssText = 'display:grid;grid-template-columns:repeat(6,1fr);gap:.5rem';

  const none = document.createElement('button');
  none.textContent = '✕';
  none.title = 'Sem ícone';
  none.style.cssText = 'padding:.5rem;border:1px solid var(--border);border-radius:6px;cursor:pointer;background:var(--bg);color:var(--text)';
  none.onclick = () => { selectIcon(''); modal.remove(); };
  grid.appendChild(none);

  for (const key of ICONS) {
    const btn = document.createElement('button');
    btn.title = key;
    btn.style.cssText = `padding:.5rem;border:1px solid var(--border);border-radius:6px;cursor:pointer;background:${currentIcon === key ? 'var(--bg-active)' : 'var(--bg)'};color:var(--text);font-size:.75rem`;
    btn.textContent = key.slice(0, 2);
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
