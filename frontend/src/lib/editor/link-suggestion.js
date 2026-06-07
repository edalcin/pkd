/**
 * link-suggestion.js — TipTap Suggestion plugin for [[ document links.
 *
 * Triggered by "[[", queries /api/search, inserts a docLink inline node.
 * Debounced at 150 ms. Falls back to client-side filtering for small trees.
 */

import { apiGet } from '../api.js'

let debounceTimer = null

function debounce(fn, ms) {
  return function (...args) {
    clearTimeout(debounceTimer)
    return new Promise(resolve => {
      debounceTimer = setTimeout(() => resolve(fn(...args)), ms)
    })
  }
}

const fetchSuggestions = debounce(async (query) => {
  // Strip trailing ] so the [Title] typing pattern works naturally
  const q = query.endsWith(']') ? query.slice(0, -1) : query
  if (!q) return []
  try {
    const results = await apiGet(`/api/search?q=${encodeURIComponent(q)}&limit=10`)
    return results.slice(0, 10)
  } catch {
    return []
  }
}, 150)

/**
 * Build the Suggestion config for TipTap.
 *
 * @param {function(items: Array, props: object): HTMLElement} renderPopup
 *   Factory that creates the dropdown DOM element.
 */
export function buildLinkSuggestion(onSelect) {
  return {
    char: '[',
    allowSpaces: true,
    startOfLine: false,

    items: ({ query }) => fetchSuggestions(query),

    render: () => {
      let popup = null
      let selectedIndex = 0
      let currentItems = []
      let currentProps = null

      function renderList(items, props) {
        if (!popup) {
          popup = document.createElement('div')
          popup.className = 'doc-link-dropdown'
          document.body.appendChild(popup)
        }

        currentItems = items
        currentProps = props

        if (!items.length) {
          popup.style.display = 'none'
          return
        }

        popup.style.display = 'block'
        popup.innerHTML = ''

        // Position near the cursor
        const { left, top, height } = props.clientRect?.() ?? {}
        if (left != null) {
          popup.style.position = 'fixed'
          popup.style.left = `${left}px`
          popup.style.top = `${top + height}px`
        }

        items.forEach((item, i) => {
          const el = document.createElement('div')
          el.className = 'doc-link-dropdown-item' + (i === selectedIndex ? ' selected' : '')
          el.innerHTML = `<span>📄</span><span>${escapeHTML(item.title)}</span>`
          el.addEventListener('mousedown', e => {
            e.preventDefault()
            selectItem(i)
          })
          popup.appendChild(el)
        })
      }

      function selectItem(index) {
        const item = currentItems[index]
        if (!item || !currentProps) return
        currentProps.command({ id: item.id, label: item.title })
        closePopup()
      }

      function closePopup() {
        if (popup) {
          popup.remove()
          popup = null
        }
      }

      return {
        onStart: props => renderList(props.items, props),
        onUpdate: props => {
          selectedIndex = 0
          renderList(props.items, props)
        },
        onKeyDown({ event }) {
          if (event.key === ']' && currentItems.length > 0) {
            selectItem(selectedIndex)
            return true
          }
          if (!currentItems.length) return false
          if (event.key === 'ArrowDown') {
            selectedIndex = (selectedIndex + 1) % currentItems.length
            renderList(currentItems, currentProps)
            return true
          }
          if (event.key === 'ArrowUp') {
            selectedIndex = (selectedIndex - 1 + currentItems.length) % currentItems.length
            renderList(currentItems, currentProps)
            return true
          }
          if (event.key === 'Enter') {
            selectItem(selectedIndex)
            return true
          }
          if (event.key === 'Escape') {
            closePopup()
            return true
          }
          return false
        },
        onExit: closePopup,
      }
    },

    command: ({ editor, range, props }) => {
      editor
        .chain()
        .focus()
        .deleteRange(range)
        .insertContent({
          type: 'docLink',
          attrs: { docId: props.id, docTitle: props.label },
        })
        .run()
    },
  }
}

function escapeHTML(str) {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}
