import { writable } from 'svelte/store'
import { apiGet, apiPost } from '../api.js'
import { loadMemories } from './memories.js'
import { loadTree } from './documents.js'

/** Flat list of Notas, already sorted by the server (favorites first, then
 *  created_at desc). Excludes archived/trashed Notas. */
export const notes = writable([])

/** Reload the Notas list from the server, filtered by tag (AND) and favorites
 *  — same query params as GET /api/tree. */
export async function loadNotes(tags = [], favoritesOnly = false) {
  const params = new URLSearchParams()
  tags.forEach(t => params.append('tag', t))
  if (favoritesOnly) params.set('favorite', '1')
  const qs = params.toString()
  notes.set(await apiGet('/api/notes' + (qs ? '?' + qs : '')))
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
