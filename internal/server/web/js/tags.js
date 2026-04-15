/**
 * tags.js — Hashtag management UI
 *
 * Provides:
 * - loadTags(): fetch GET /api/tags and return array of {id, name, count}
 * - renderTagFilter(container, onFilter): draws a chip list for tree filtering
 * - setDocumentTags(docId, tagNames): calls PUT /api/documents/{id}/tags
 */
import { apiFetch } from './app.js';
import { loadTree } from './tree.js';

let activeTags = new Set();

export async function loadTags() {
  const res = await apiFetch('/api/tags');
  if (!res.ok) return [];
  return res.json();
}

export async function renderTagFilter(container) {
  const tags = await loadTags();
  container.innerHTML = '';
  if (!tags.length) return;

  const wrapper = document.createElement('div');
  wrapper.style.cssText = 'padding:.5rem .75rem;border-bottom:1px solid var(--border)';
  const label = document.createElement('div');
  label.textContent = 'Filter by tag';
  label.style.cssText = 'font-size:.75rem;color:var(--text-muted);margin-bottom:.375rem';
  wrapper.appendChild(label);

  const chips = document.createElement('div');
  chips.style.cssText = 'display:flex;flex-wrap:wrap;gap:.25rem';

  for (const tag of tags) {
    const chip = document.createElement('button');
    chip.textContent = '#' + tag.name;
    chip.dataset.name = tag.name;
    chip.style.cssText = 'padding:.2rem .5rem;border-radius:12px;border:1px solid var(--border);background:var(--bg);color:var(--text);cursor:pointer;font-size:.75rem';
    chip.addEventListener('click', () => {
      if (activeTags.has(tag.name)) {
        activeTags.delete(tag.name);
        chip.style.background = 'var(--bg)';
        chip.style.color = 'var(--text)';
      } else {
        activeTags.add(tag.name);
        chip.style.background = 'var(--accent)';
        chip.style.color = 'var(--accent-text)';
      }
      loadTree([...activeTags]);
    });
    chips.appendChild(chip);
  }

  wrapper.appendChild(chips);
  container.prepend(wrapper);
}

export async function setDocumentTags(docId, tagNames) {
  await apiFetch(`/api/documents/${docId}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tags: tagNames }),
  });
}
