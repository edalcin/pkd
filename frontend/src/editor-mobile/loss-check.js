/**
 * loss-check.js — silent content-loss detection (ADR pkdMobile 0002, §Proteção).
 *
 * After setContent(html), the mobile bundle's schema may not know an extension
 * the PWA used (a future PWA-only node/mark, or garbage HTML). Compare the
 * HTML fed in against getHTML() after the round-trip through TipTap's schema:
 *
 *  - plain text (whitespace-normalized) must be unchanged.
 *  - every element tag present in the original must appear at least as many
 *    times in the round-tripped HTML (an element can gain wrapper tags, but an
 *    existing one must never disappear).
 *
 * This generalizes the ADR's named list (img, table, a, li, span[data-*], pre)
 * to a full per-tag histogram: any tag — named or not — that silently drops
 * out is loss. It also catches inputs containing an element the schema has no
 * parseHTML rule for at all (ProseMirror parses through unknown wrapper tags
 * and keeps their text, so the wrapper's own count is what reveals the drop).
 */

function tagHistogram(container) {
  const counts = {}
  container.querySelectorAll('*').forEach(el => {
    const tag = el.tagName.toLowerCase()
    counts[tag] = (counts[tag] || 0) + 1
  })
  return counts
}

function plainText(container) {
  return (container.textContent || '').replace(/\s+/g, ' ').trim()
}

/**
 * @param {string} originalHTML - HTML passed to setContent()
 * @param {string} roundTrippedHTML - editor.getHTML() right after
 * @returns {{ ok: boolean, details: { textOk: boolean, missing: object } }}
 */
export function checkContentLoss(originalHTML, roundTrippedHTML) {
  const origEl = document.createElement('div')
  origEl.innerHTML = originalHTML || ''
  const newEl = document.createElement('div')
  newEl.innerHTML = roundTrippedHTML || ''

  const origCounts = tagHistogram(origEl)
  const newCounts = tagHistogram(newEl)

  const missing = {}
  for (const tag in origCounts) {
    const before = origCounts[tag]
    const after = newCounts[tag] || 0
    if (after < before) missing[tag] = { before, after }
  }

  const textOk = plainText(origEl) === plainText(newEl)
  const ok = textOk && Object.keys(missing).length === 0
  return { ok, details: { textOk, missing } }
}
