/**
 * nav.js — hash navigation that REPLACES the current history entry.
 *
 * App.svelte keeps its own back/forward stack (navHistory) on every
 * hashchange. replaceHash() flags the next hashchange so App overwrites the
 * current entry instead of pushing a new one, in step with the browser's own
 * history (location.replace). Used by the Mural de Notas: #/doc/{id} → #/notas/{id}
 * redirect, closing the Nota modal, and leaving it after delete/archive/convert.
 */
export const nav = { replaceNext: false }

export function replaceHash(path) {
  if (window.location.hash.slice(1) === path) return // no hashchange would fire
  nav.replaceNext = true
  window.location.replace('#' + path)
}
