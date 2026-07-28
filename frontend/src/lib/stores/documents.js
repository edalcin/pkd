import { writable, get } from 'svelte/store'
import { apiGet, apiPost, apiPut, apiDelete } from '../api.js'

/** Full tree of DocumentTreeNode objects. */
export const tree = writable([])

/** Current document tree view mode: 'active' | 'archived' | 'all'. Resets to 'active' on page load. */
export const viewMode = writable('active')

/** View mode saved before a search is initiated; restored when search is cleared. */
let preSearchViewMode = 'active'

/** Currently open document (full Document object). */
export const activeDoc = writable(null)

/** Active document ID (from hash routing). */
export const activeDocId = writable(null)

/** Tag filter applied to the tree. Array of tag names. */
export const tagFilter = writable([])

/** Whether the tree is filtered to show only favorites. */
export const favoriteFilter = writable(false)

/** Text filter applied to the tree (searches title, body, external link titles). */
export const textFilter = writable('')

/** Whether any data is loading. */
export const loading = writable(false)

/** Controls expand/collapse of all tree nodes. 'expand' | 'collapse' | null */
export const treeExpansionSignal = writable(null)

/** Incremented when the text filter is cleared, to reveal and scroll to the active node. */
export const revealActiveSignal = writable(0)

/** Incremented whenever a link is created from the sidebar, so Editor reloads. */
export const linksRefreshSignal = writable(0)

/** Set to a doc ID when that document's body was externally updated (e.g. admin import).
 *  Editor watches this and reloads content via setContent if the doc is currently open. */
export const docBodyRefreshedSignal = writable(null)

/** When set to a doc ID, Editor should focus the title input after loading that doc. */
export const focusTitleForDocId = writable(null)

/** Load the document tree, optionally filtered by tags, favorites, and/or text query. */
export async function loadTree(tags = get(tagFilter), favorites = get(favoriteFilter), q = get(textFilter)) {
  const prevQ = get(textFilter)
  tagFilter.set(tags)
  favoriteFilter.set(favorites)
  textFilter.set(q)

  // Search always spans all documents (active + archived).
  // Save the current view before the first search; restore only when search
  // is actually being cleared (prevQ was non-empty), not on every call with q=''.
  const currentView = get(viewMode)
  if (q && currentView !== 'all') {
    preSearchViewMode = currentView
    viewMode.set('all')
  } else if (!q && prevQ && currentView === 'all' && preSearchViewMode !== 'all') {
    viewMode.set(preSearchViewMode)
  }

  const params = new URLSearchParams()
  const view = get(viewMode)
  if (view && view !== 'active') params.set('view', view)
  tags.forEach(t => params.append('tag', t))
  if (favorites) params.set('favorite', '1')
  if (q) params.set('q', q)
  const qs = params.toString()
  tree.set(await apiGet('/api/tree' + (qs ? '?' + qs : '')))
}

/** Toggle the favorite state of a document. */
export async function toggleFavorite(id) {
  const doc = await apiPost(`/api/documents/${id}/favorite`, {})
  if (get(favoriteFilter)) {
    await loadTree()
  } else {
    tree.update(nodes => updateTreeNode(nodes, id, { is_favorite: doc.is_favorite }))
  }
  return doc
}

/** Toggle the locked state of a document. */
export async function toggleLock(id) {
  const doc = await apiPost(`/api/documents/${id}/lock`, {})
  tree.update(nodes => updateTreeNode(nodes, id, { locked: doc.locked }))
  return doc
}

/** Encrypt a document at rest (Protect). Returns updated doc with plaintext body still visible. */
export async function protectDoc(id) { return apiPost(`/api/documents/${id}/protect`) }

/** Decrypt a document back to plaintext (Unprotect). Requires the doc to be unlocked in this session. */
export async function unprotectDoc(id) { return apiPost(`/api/documents/${id}/unprotect`) }

/** Request a fresh unlock code by e-mail for an encrypted document. Returns null on 204 (already unlocked). */
export async function requestDocCode(id) { return apiPost(`/api/documents/${id}/unlock/request`) }

/** Submit an unlock code/challenge to decrypt a document for the current session. */
export async function unlockDoc(id, challengeId, code) { return apiPost(`/api/documents/${id}/unlock`, { challenge_id: challengeId, code }) }

/** Archive a document. Triggers a tree reload so it disappears from the current view. */
export async function archiveDoc(id) {
  const doc = await apiPost(`/api/documents/${id}/archive`, {})
  await loadTree()
  return doc
}

/** Unarchive a document. Triggers a tree reload so it reappears in the active view. */
export async function unarchiveDoc(id) {
  const doc = await apiPost(`/api/documents/${id}/unarchive`, {})
  await loadTree()
  return doc
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
  focusTitleForDocId.set(doc.id)
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

/** Move a document to a new parent (nest as child). */
export async function moveDoc(id, newParentId) {
  await apiPost(`/api/documents/${id}/move`, { new_parent_id: newParentId })
  await loadTree()
}

/** Reorder a document within its parent. beforeId=null appends at end. */
export async function reorderDoc(id, parentId, beforeId) {
  await apiPost(`/api/documents/${id}/reorder`, { parent_id: parentId ?? null, before_id: beforeId ?? null })
  await loadTree()
}

/** List all version snapshots for a document (no body_html). */
export async function listVersions(docId) {
  return apiGet(`/api/documents/${docId}/versions`)
}

/** Fetch a single version snapshot including body_html. */
export async function getVersion(docId, versionId) {
  return apiGet(`/api/documents/${docId}/versions/${versionId}`)
}

/** Delete a single version snapshot. */
export async function deleteVersion(docId, versionId) {
  await apiDelete(`/api/documents/${docId}/versions/${versionId}`)
}

/** Restore a snapshot as the current document content.
 *  currentVersion is the optimistic-lock token from the active document. */
export async function restoreVersion(docId, versionId, currentVersion) {
  const updated = await apiPost(
    `/api/documents/${docId}/versions/${versionId}/restore`,
    { version: currentVersion },
  )
  activeDoc.set(updated)
  tree.update(nodes => updateTreeNode(nodes, docId, { title: updated.title, icon: updated.icon }))
  return updated
}

/** Sort the entire tree by criterion: "alpha" | "created" | "updated". */
export async function sortTree(by) {
  await apiPost('/api/tree/sort', { by })
  await loadTree()
}

/** Returns the ID of the sibling immediately after afterId at the given parent level. */
export function findNextSiblingId(afterId, parentId) {
  const nodes = get(tree)
  const siblings = parentId == null ? nodes : (findChildrenOf(nodes, parentId) ?? [])
  const idx = siblings.findIndex(n => n.id === afterId)
  if (idx === -1 || idx === siblings.length - 1) return null
  return siblings[idx + 1].id
}

function findChildrenOf(nodes, parentId) {
  for (const n of nodes) {
    if (n.id === parentId) return n.children || []
    if (n.children?.length) {
      const found = findChildrenOf(n.children, parentId)
      if (found !== null) return found
    }
  }
  return null
}

// Recursively update a tree node's fields
function updateTreeNode(nodes, id, fields) {
  return nodes.map(n => {
    if (n.id === id) return { ...n, ...fields }
    if (n.children?.length) return { ...n, children: updateTreeNode(n.children, id, fields) }
    return n
  })
}
