/**
 * editor.js — PKD rich document editor (TipTap v2)
 *
 * TipTap é carregado estaticamente via <script src="/vendor/tiptap/tiptap.min.js">
 * no index.html. O bundle expõe window.PKDEditor com o factory create().
 */

import { apiFetch } from './app.js';

const ICONS = ['document','folder','star','bookmark','tag','image','link',
               'code','book','idea','note','calendar','task','archive','heart','flag'];

let editor        = null;   // instância TipTap
let currentDocId  = null;
let currentVersion = null;
let currentIcon   = '';
let saveTimer     = null;
let offline       = false;

/* ── Init ─────────────────────────────────────────────────── */
export function initEditor() {
  document.addEventListener('pkd:offline', () => {
    offline = true;
    if (editor) editor.setEditable(false);
    document.getElementById('offline-banner')?.classList.remove('hidden');
  });

  document.getElementById('conflict-overwrite')?.addEventListener('click', forceOverwrite);
  document.getElementById('conflict-reload')?.addEventListener('click', reloadStored);
  document.getElementById('doc-icon-btn')?.addEventListener('click', openIconPicker);

  document.addEventListener('pkd:docDeselected', () => {
    currentDocId = null;
    currentVersion = null;
    destroyEditor();
    const t = document.getElementById('doc-title');
    if (t) t.value = '';
    const iconBtn = document.getElementById('doc-icon-btn');
    if (iconBtn) iconBtn.textContent = '📄';
  });
}

/* ── Abrir documento ──────────────────────────────────────── */
export async function openDocument(id) {
  clearSaveTimer();
  const res = await apiFetch(`/api/documents/${id}`);
  if (!res.ok) return;
  const doc = await res.json();

  currentDocId   = doc.id;
  currentVersion = doc.version;
  currentIcon    = doc.icon || '';

  const titleEl = document.getElementById('doc-title');
  if (titleEl) {
    titleEl.value = doc.title;
    titleEl.oninput = () => scheduleSave();
  }

  const iconBtn = document.getElementById('doc-icon-btn');
  if (iconBtn) iconBtn.textContent = currentIcon || '📄';

  mountEditor(doc.body_html || '');
}

/* ── Montar TipTap ────────────────────────────────────────── */
function mountEditor(initialContent) {
  const container = document.getElementById('editor');
  const errorEl   = document.getElementById('editor-error');

  if (!container) return;
  if (errorEl) { errorEl.textContent = ''; errorEl.classList.add('hidden'); }

  // Destruir instância anterior
  destroyEditor();

  if (typeof window.PKDEditor === 'undefined') {
    showEditorError('TipTap não disponível. Recarregue a página (Ctrl+Shift+R).');
    return;
  }

  try {
    editor = window.PKDEditor.create(container, {
      content: initialContent,
      editable: !offline,
      placeholder: 'Comece a escrever…',
      onUpdate: () => { if (!offline) scheduleSave(); },
    });

    // Suporte a upload de imagem por paste/drop
    container.addEventListener('paste', handleImagePaste, { once: false });
    container.addEventListener('drop',  handleImageDrop,  { once: false });

    // Adicionar toolbar de formatação
    renderToolbar();

    // Suporte a redimensionamento de imagens via click
    container.addEventListener('click', handleImageClick);

  } catch (err) {
    console.error('TipTap mount error:', err);
    showEditorError(`Erro ao carregar editor: ${err.message}`);
  }
}

function destroyEditor() {
  if (editor) {
    try { editor.destroy(); } catch (_) {}
    editor = null;
  }
}

/* ── Toolbar ──────────────────────────────────────────────── */
function renderToolbar() {
  let toolbar = document.getElementById('tiptap-toolbar');
  if (toolbar) return; // já existe

  toolbar = document.createElement('div');
  toolbar.id = 'tiptap-toolbar';
  toolbar.setAttribute('role', 'toolbar');
  toolbar.setAttribute('aria-label', 'Formatação');

  const buttons = [
    { cmd: 'toggleBold',          label: '<b>B</b>',  title: 'Negrito' },
    { cmd: 'toggleItalic',        label: '<i>I</i>',  title: 'Itálico' },
    { cmd: 'toggleUnderline',     label: '<u>S</u>',  title: 'Sublinhado' },
    { cmd: 'toggleStrike',        label: '<s>S</s>',  title: 'Riscado' },
    { sep: true },
    { cmd: 'toggleHeading:1',     label: 'H1',        title: 'Título 1' },
    { cmd: 'toggleHeading:2',     label: 'H2',        title: 'Título 2' },
    { cmd: 'toggleHeading:3',     label: 'H3',        title: 'Título 3' },
    { sep: true },
    { cmd: 'toggleBulletList',    label: '• Lista',   title: 'Lista' },
    { cmd: 'toggleOrderedList',   label: '1. Lista',  title: 'Lista numerada' },
    { cmd: 'toggleBlockquote',    label: '❝',         title: 'Citação' },
    { cmd: 'toggleCodeBlock',     label: '</>',       title: 'Bloco de código' },
    { sep: true },
    { cmd: 'insertTable',         label: '⊞',         title: 'Tabela' },
    { cmd: 'setLink',             label: '🔗',         title: 'Link' },
    { cmd: 'uploadImage',         label: '🖼',         title: 'Inserir imagem' },
  ];

  for (const btn of buttons) {
    if (btn.sep) {
      const sep = document.createElement('span');
      sep.className = 'toolbar-sep';
      toolbar.appendChild(sep);
      continue;
    }
    const b = document.createElement('button');
    b.innerHTML = btn.label;
    b.title = btn.title;
    b.type = 'button';
    b.className = 'toolbar-btn';
    b.addEventListener('click', () => execCmd(btn.cmd));
    toolbar.appendChild(b);
  }

  // Inserir toolbar ANTES do container do editor
  const container = document.getElementById('editor');
  container.parentNode.insertBefore(toolbar, container);
}

function execCmd(cmd) {
  if (!editor) return;
  const chain = editor.chain().focus();
  switch (cmd) {
    case 'toggleBold':         chain.toggleBold().run(); break;
    case 'toggleItalic':       chain.toggleItalic().run(); break;
    case 'toggleUnderline':    chain.toggleUnderline().run(); break;
    case 'toggleStrike':       chain.toggleStrike().run(); break;
    case 'toggleHeading:1':    chain.toggleHeading({ level: 1 }).run(); break;
    case 'toggleHeading:2':    chain.toggleHeading({ level: 2 }).run(); break;
    case 'toggleHeading:3':    chain.toggleHeading({ level: 3 }).run(); break;
    case 'toggleBulletList':   chain.toggleBulletList().run(); break;
    case 'toggleOrderedList':  chain.toggleOrderedList().run(); break;
    case 'toggleBlockquote':   chain.toggleBlockquote().run(); break;
    case 'toggleCodeBlock':    chain.toggleCodeBlock().run(); break;
    case 'insertTable':
      chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(); break;
    case 'setLink': {
      const url = prompt('URL do link:');
      if (url) chain.setLink({ href: url }).run();
      break;
    }
    case 'uploadImage':
      triggerImageUpload(); break;
  }
}

/* ── Upload de imagem ─────────────────────────────────────── */
function triggerImageUpload() {
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = 'image/*';
  input.onchange = async () => {
    if (input.files?.[0]) await uploadImageFile(input.files[0]);
  };
  input.click();
}

async function handleImagePaste(e) {
  const items = e.clipboardData?.items;
  if (!items) return;
  for (const item of items) {
    if (item.type.startsWith('image/')) {
      e.preventDefault();
      await uploadImageFile(item.getAsFile());
      return;
    }
  }
}

async function handleImageDrop(e) {
  const files = e.dataTransfer?.files;
  if (!files?.length) return;
  for (const file of files) {
    if (file.type.startsWith('image/')) {
      e.preventDefault();
      e.stopPropagation();
      await uploadImageFile(file);
      return;
    }
  }
}

async function uploadImageFile(file) {
  if (!currentDocId || !editor) return;
  const fd = new FormData();
  fd.append('file', file);
  const res = await apiFetch(`/api/documents/${currentDocId}/attachments`, {
    method: 'POST',
    body: fd,
  });
  if (!res.ok) { alert('Erro ao fazer upload da imagem.'); return; }
  const data = await res.json();
  const url  = data.url || data.default;
  if (url) {
    editor.chain().focus().setImage({ src: url, alt: file.name }).run();
  }
}

/* ── Redimensionamento de imagem ─────────────────────────── */
let resizingImg = null;

function handleImageClick(e) {
  const img = e.target.closest('img');
  if (!img) { clearImageResize(); return; }
  if (resizingImg === img) return;
  clearImageResize();
  resizingImg = img;
  addResizeHandle(img);
}

function addResizeHandle(img) {
  img.style.cursor = 'default';
  img.style.outline = '2px solid var(--accent)';

  const handle = document.createElement('div');
  handle.id = 'pkd-resize-handle';
  handle.style.cssText = `
    position: absolute;
    width: 10px; height: 10px;
    background: var(--accent);
    border-radius: 50%;
    cursor: se-resize;
    z-index: 100;
  `;

  function positionHandle() {
    const rect = img.getBoundingClientRect();
    const pRect = img.closest('.ProseMirror, #editor').getBoundingClientRect();
    handle.style.left = (rect.right - pRect.left - 6) + 'px';
    handle.style.top  = (rect.bottom - pRect.top - 6) + 'px';
  }

  const editorEl = img.closest('.ProseMirror, #editor') || document.getElementById('editor');
  editorEl.style.position = 'relative';
  editorEl.appendChild(handle);
  positionHandle();

  let startX, startW;
  handle.addEventListener('mousedown', e => {
    e.preventDefault();
    startX = e.clientX;
    startW = img.offsetWidth;
    const onMove = e2 => {
      const newW = Math.max(50, startW + e2.clientX - startX);
      img.style.width = newW + 'px';
      img.setAttribute('width', newW);
      positionHandle();
    };
    const onUp = () => {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      // Persiste a largura no modelo do TipTap
      if (editor) {
        editor.chain().focus().updateAttributes('image', {
          width: img.offsetWidth + 'px',
        }).run();
      }
      scheduleSave();
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  });

  // Remover handle ao clicar fora
  setTimeout(() => {
    document.addEventListener('click', e => {
      if (!e.target.closest('img') && !e.target.closest('#pkd-resize-handle')) {
        clearImageResize();
      }
    }, { once: true });
  }, 0);
}

function clearImageResize() {
  if (resizingImg) {
    resizingImg.style.outline = '';
    resizingImg.style.cursor  = '';
    resizingImg = null;
  }
  document.getElementById('pkd-resize-handle')?.remove();
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

  const titleEl  = document.getElementById('doc-title');
  const title    = titleEl?.value.trim() || 'Sem título';
  const bodyHTML = editor.getHTML();
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
  if (editor) editor.commands.setContent(stored.body_html || '');
}

/* ── Helpers ─────────────────────────────────────────────── */
function showEditorError(msg) {
  const el = document.getElementById('editor-error');
  if (el) { el.textContent = msg; el.classList.remove('hidden'); }
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
  none.textContent = '✕'; none.title = 'Sem ícone';
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
