import { writable } from 'svelte/store'
import { apiGet, apiPut } from '../api.js'

/** All tags with document counts: [{id, name, count}] */
export const tags = writable([])

export async function loadTags() {
  tags.set(await apiGet('/api/tags'))
}

/** Replace all tags on a document. */
export async function setDocumentTags(docId, tagNames) {
  // Optimistic update: new tags appear in sidebar immediately
  tags.update(current => {
    const existing = new Set(current.map(t => t.name))
    const newEntries = tagNames
      .filter(n => !existing.has(n))
      .map(n => ({ name: n, count: 1, color: null }))
    return newEntries.length ? [...current, ...newEntries] : current
  })
  await apiPut(`/api/documents/${docId}/tags`, { tags: tagNames })
  await loadTags()
}
