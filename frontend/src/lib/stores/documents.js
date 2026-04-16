import { writable, get } from 'svelte/store'
import { apiGet, apiPost, apiPut, apiDelete } from '../api.js'

/** Full tree of DocumentTreeNode objects. */
export const tree = writable([])

/** Currently open document (full Document object). */
export const activeDoc = writable(null)

/** Active document ID (from hash routing). */
export const activeDocId = writable(null)

/** Tag filter applied to the tree. Array of tag names. */
export const tagFilter = writable([])

/** Whether any data is loading. */
export const loading = writable(false)

/** Load the document tree, optionally filtered by tags. */
export async function loadTree(tags = get(tagFilter)) {
  tagFilter.set(tags)
  const params = new URLSearchParams()
  tags.forEach(t => params.append('tag', t))
  const url = '/api/tree' + (tags.length ? '?' + params : '')
  tree.set(await apiGet(url))
}

/** Load a single document by ID. */
export async function loadDoc(id) {
  activeDocId.set(id)
  const doc = await apiGet(`/api/documents/${id}`)
  activeDoc.set(doc)
  return doc
}

/** Create a new document (optionally under parentId). */
export async function createDoc(parentId = null, title = 'Untitled') {
  const doc = await apiPost('/api/documents', { parent_id: parentId, title })
  await loadTree()
  return doc
}

/** Save document content. Returns the saved doc or a conflict object. */
export async function saveDoc(id, { version, title, body_html, body_text, icon }) {
  const result = await apiPut(`/api/documents/${id}`, {
    version,
    title,
    body_html,
    body_text,
    icon,
  })
  if (result?._conflict) return result
  activeDoc.set(result)
  // Update tree node title/icon without full reload
  tree.update(nodes => updateTreeNode(nodes, id, { title, icon }))
  return result
}

/** Soft-delete a document (move to trash). */
export async function trashDoc(id) {
  await apiDelete(`/api/documents/${id}`)
  activeDoc.set(null)
  activeDocId.set(null)
  await loadTree()
}

/** Restore a document from trash. */
export async function restoreDoc(id) {
  await apiPost(`/api/documents/${id}/restore`)
  await loadTree()
}

/** Move a document to a new parent. */
export async function moveDoc(id, newParentId) {
  await apiPost(`/api/documents/${id}/move`, { new_parent_id: newParentId })
  await loadTree()
}

// Recursively update a tree node's fields
function updateTreeNode(nodes, id, fields) {
  return nodes.map(n => {
    if (n.id === id) return { ...n, ...fields }
    if (n.children?.length) return { ...n, children: updateTreeNode(n.children, id, fields) }
    return n
  })
}
