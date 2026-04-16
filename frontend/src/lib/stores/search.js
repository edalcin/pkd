import { writable } from 'svelte/store'
import { apiGet } from '../api.js'

export const searchQuery = writable('')
export const searchResults = writable([])
export const searching = writable(false)

let debounceTimer = null

/**
 * Search with a 150 ms debounce.
 * @param {string} q
 */
export function debouncedSearch(q) {
  searchQuery.set(q)
  clearTimeout(debounceTimer)
  if (!q.trim()) {
    searchResults.set([])
    return
  }
  debounceTimer = setTimeout(() => runSearch(q), 150)
}

export async function runSearch(q) {
  if (!q.trim()) return
  searching.set(true)
  try {
    const results = await apiGet(`/api/search?q=${encodeURIComponent(q)}`)
    searchResults.set(results)
  } catch {
    searchResults.set([])
  } finally {
    searching.set(false)
  }
}

export function clearSearch() {
  searchQuery.set('')
  searchResults.set([])
  clearTimeout(debounceTimer)
}
