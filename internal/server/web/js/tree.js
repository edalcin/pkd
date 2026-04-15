/**
 * tree.js — PKD hierarchical document tree
 *
 * Features:
 * - Render nested DocumentTreeNode array from GET /api/tree
 * - Click-to-select opens document in editor
 * - Inline rename via double-click
 * - Context menu (right-click / long-press): New Child, Rename, Delete
 * - Drag-and-drop move with circular-move guard (also enforced server-side)
 * - Tag filter integration: passes ?tag= params to GET /api/tree
 */

import { apiFetch } from './app.js';

let onSelectCallback = () => {};
let activeId = null;
let tagFilter = [];

export function initTree({ onSelect }) {
  onSelectCallback = onSelect;
}

export async function loadTree(tags = tagFilter) {
  tagFilter = tags;
  const params = new URLSearchParams();
  tags.forEach(t => params.append('tag', t));
  const url = '/api/tree' + (tags.length ? '?' + params : '');
  const res = await apiFetch(url);
  if (!res.ok) return;
  const nodes = await res.json();
  renderTree(nodes);
}

/* ── Render ──────────────────────────────────────────────── */
function renderTree(nodes) {
  const container = document.getElementById('tree');
  if (!container) return;
  container.innerHTML = '';
  const ul = buildList(nodes, 0);
  container.appendChild(ul);
}

function buildList(nodes, depth) {
  const ul = document.createElement('ul');
  ul.style.listStyle = 'none';
  for (const node of nodes) {
    ul.appendChild(buildItem(node, depth));
  }
  return ul;
}

function buildItem(node, depth) {
  const li = document.createElement('li');

  const row = document.createElement('div');
  row.className = 'tree-item' + (node.id === activeId ? ' active' : '');
  row.dataset.id = node.id;
  row.dataset.parentId = node.parent_id ?? '';
  row.draggable = true;
  row.title = node.title;
  row.style.paddingLeft = `${0.75 + depth * 1}rem`;

  // Toggle button (only shown if has children)
  const toggle = document.createElement('span');
  toggle.className = 'toggle';
  toggle.textContent = node.children?.length ? '▶' : '';
  row.appendChild(toggle);

  // Icon
  const icon = document.createElement('span');
  icon.className = 'icon';
  icon.textContent = node.icon || '📄';
  row.appendChild(icon);

  // Label
  const label = document.createElement('span');
  label.className = 'label';
  label.textContent = node.title;
  row.appendChild(label);

  // Children container
  const childrenDiv = document.createElement('div');
  childrenDiv.className = 'tree-children';
  let expanded = true;

  if (node.children?.length) {
    childrenDiv.appendChild(buildList(node.children, depth + 1));
    toggle.style.cursor = 'pointer';
    toggle.addEventListener('click', e => {
      e.stopPropagation();
      expanded = !expanded;
      toggle.textContent = expanded ? '▾' : '▶';
      childrenDiv.style.display = expanded ? '' : 'none';
    });
    toggle.textContent = '▾';
  }

  // Select
  row.addEventListener('click', () => {
    document.querySelectorAll('.tree-item.active').forEach(el => el.classList.remove('active'));
    row.classList.add('active');
    activeId = node.id;
    onSelectCallback(node.id);
  });

  // Double-click → inline rename
  label.addEventListener('dblclick', e => {
    e.stopPropagation();
    startRename(label, node);
  });

  // Context menu
  row.addEventListener('contextmenu', e => {
    e.preventDefault();
    showContextMenu(e.clientX, e.clientY, node);
  });

  // Drag-and-drop
  row.addEventListener('dragstart', e => {
    e.dataTransfer.setData('text/plain', String(node.id));
    e.dataTransfer.effectAllowed = 'move';
  });
  row.addEventListener('dragover', e => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    row.style.outline = '2px dashed var(--accent)';
  });
  row.addEventListener('dragleave', () => { row.style.outline = ''; });
  row.addEventListener('drop', async e => {
    e.preventDefault();
    row.style.outline = '';
    const draggedId = parseInt(e.dataTransfer.getData('text/plain'));
    if (!draggedId || draggedId === node.id) return;
    const res = await apiFetch(`/api/documents/${draggedId}/move`, {
      method: 'POST',
      body: JSON.stringify({ new_parent_id: node.id }),
    });
    if (res.status === 400) {
      alert('Cannot move: circular move detected.');
    }
    await loadTree();
  });

  li.appendChild(row);
  li.appendChild(childrenDiv);
  return li;
}

/* ── Inline rename ───────────────────────────────────────── */
function startRename(label, node) {
  const input = document.createElement('input');
  input.type = 'text';
  input.value = node.title;
  input.style.cssText = 'width:100%;border:1px solid var(--accent);border-radius:4px;padding:0 4px;font-size:inherit;background:var(--bg);color:var(--text)';
  label.replaceWith(input);
  input.focus();
  input.select();

  async function commit() {
    const newTitle = input.value.trim() || node.title;
    if (newTitle !== node.title) {
      await apiFetch(`/api/documents/${node.id}`, {
        method: 'PUT',
        body: JSON.stringify({ version: node.version, title: newTitle, body_html: '', body_text: '', icon: node.icon || '' }),
      });
    }
    await loadTree();
  }
  input.addEventListener('blur', commit);
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); input.blur(); }
    if (e.key === 'Escape') { input.value = node.title; input.blur(); }
  });
}

/* ── Context menu ────────────────────────────────────────── */
function showContextMenu(x, y, node) {
  removeContextMenu();
  const menu = document.createElement('div');
  menu.id = 'tree-context-menu';
  menu.style.cssText = `position:fixed;left:${x}px;top:${y}px;background:var(--bg-card);border:1px solid var(--border);border-radius:6px;padding:.25rem 0;z-index:1000;min-width:140px;box-shadow:var(--shadow)`;

  const items = [
    { label: 'New child', action: () => newChild(node.id) },
    { label: 'Rename', action: () => {
      removeContextMenu();
      const labelEl = document.querySelector(`.tree-item[data-id="${node.id}"] .label`);
      if (labelEl) startRename(labelEl, node);
    }},
    { label: 'Delete', action: () => deleteNode(node), danger: true },
  ];

  for (const item of items) {
    const el = document.createElement('div');
    el.textContent = item.label;
    el.style.cssText = `padding:.375rem .75rem;cursor:pointer;font-size:.875rem;color:${item.danger ? 'var(--danger)' : 'var(--text)'}`;
    el.addEventListener('mouseenter', () => { el.style.background = 'var(--bg-hover)'; });
    el.addEventListener('mouseleave', () => { el.style.background = ''; });
    el.addEventListener('click', () => { removeContextMenu(); item.action(); });
    menu.appendChild(el);
  }

  document.body.appendChild(menu);
  setTimeout(() => document.addEventListener('click', removeContextMenu, { once: true }), 0);
}

function removeContextMenu() {
  document.getElementById('tree-context-menu')?.remove();
}

/* ── Actions ─────────────────────────────────────────────── */
async function newChild(parentId) {
  const res = await apiFetch('/api/documents', {
    method: 'POST',
    body: JSON.stringify({ parent_id: parentId, title: 'Untitled' }),
  });
  if (res.ok) {
    const doc = await res.json();
    await loadTree();
    onSelectCallback(doc.id);
  }
}

async function deleteNode(node) {
  if (!confirm(`Delete "${node.title}"? It will go to Trash.`)) return;
  const res = await apiFetch(`/api/documents/${node.id}`, { method: 'DELETE' });
  if (res.ok) {
    if (activeId === node.id) {
      activeId = null;
      document.dispatchEvent(new CustomEvent('pkd:docDeselected'));
    }
    await loadTree();
  }
}
