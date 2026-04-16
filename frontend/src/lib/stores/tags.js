import { writable } from 'svelte/store'
import { apiGet, apiPut } from '../api.js'

/** All tags with document counts: [{id, name, count}] */
export const tags = writable([])

export async function loadTags() {
  tags.set(await apiGet('/api/tags'))
}

/** Replace all tags on a document. */
export async function setDocumentTags(docId, tagNames) {
  await apiPut(`/api/documents/${docId}/tags`, { tags: tagNames })
  await loadTags()
}
