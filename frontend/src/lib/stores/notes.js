import { writable, get } from 'svelte/store'
import { apiGet, apiPost } from '../api.js'
import { loadMemories } from './memories.js'
import { loadTree, tagFilter, favoriteFilter, viewMode } from './documents.js'

/** Flat list of Notas, already sorted by the server (favorites first, then
 *  created_at desc). Honors the sidebar viewMode (active/archived/all). */
export const notes = writable([])

let loadSeq = 0

/** Reload the Notas list, filtered by tag (AND) and favorites — same query
 *  params as GET /api/tree. Defaults to the active sidebar filters (like
 *  loadTree), so reloads after rename/archive/trash keep the filter (Q5). */
export async function loadNotes(tags = get(tagFilter), favoritesOnly = get(favoriteFilter), view = get(viewMode)) {
  const seq = ++loadSeq
  const params = new URLSearchParams()
  if (view && view !== 'active') params.set('view', view)
  tags.forEach(t => params.append('tag', t))
  if (favoritesOnly) params.set('favorite', '1')
  const qs = params.toString()
  const list = await apiGet('/api/notes' + (qs ? '?' + qs : ''))
  if (seq === loadSeq) notes.set(list) // a slower, older response must not overwrite a newer filter
}

/** Create a new Nota. Only title is required (Q6). */
export async function createNote(title, { content, tags, favorite } = {}) {
  const doc = await apiPost('/api/notes', { title, content, tags, favorite })
  await loadNotes()
  return doc
}

/** Convert a Nota into a Documento (one-way, Q3), positioned as a child of
 *  parentId before beforeId (null beforeId appends at end; null parentId is
 *  root). Refreshes both the Notas list and the normal tree, since the
 *  converted doc now belongs there. */
export async function convertNoteToDocument(id, parentId, beforeId) {
  const doc = await apiPost(`/api/notes/${id}/convert`, {
    to: 'document',
    parent_id: parentId ?? null,
    before_id: beforeId ?? null,
  })
  await loadNotes()
  await loadTree()
  return doc
}

/** Convert a Nota into a Memória (one-way, Q3).
 *  date: {year, month?, day?, hour?, minute?, period?} */
export async function convertNoteToMemory(id, date) {
  const doc = await apiPost(`/api/notes/${id}/convert`, { to: 'memory', date })
  await loadNotes()
  await loadMemories()
  return doc
}
