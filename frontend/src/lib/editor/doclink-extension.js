import { Node, mergeAttributes } from '@tiptap/core'
import Suggestion from '@tiptap/suggestion'
import { buildLinkSuggestion } from './link-suggestion.js'

/**
 * docLink — TipTap inline node for bidirectional document links.
 *
 * Rendered as: <span data-doc-link="42" class="doc-link">Title</span>
 * The Go backend parses data-doc-link attributes on save to maintain
 * the document_links table.
 */
export const DocLink = Node.create({
  name: 'docLink',
  group: 'inline',
  inline: true,
  atom: true, // treated as a single unit by the cursor

  addAttributes() {
    return {
      docId: {
        default: null,
        parseHTML: el => el.getAttribute('data-doc-link'),
        renderHTML: attrs => ({ 'data-doc-link': attrs.docId }),
      },
      docTitle: {
        default: '',
        parseHTML: el => el.textContent || el.getAttribute('data-doc-title') || '',
        renderHTML: attrs => ({ 'data-doc-title': attrs.docTitle }),
      },
      broken: {
        default: false,
        parseHTML: el => el.classList.contains('broken'),
        renderHTML: attrs => (attrs.broken ? { class: 'doc-link broken' } : { class: 'doc-link' }),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'span[data-doc-link]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['span', mergeAttributes(HTMLAttributes), HTMLAttributes['data-doc-title'] || '?']
  },

  renderText({ node }) {
    return `[${node.attrs.docTitle || node.attrs.docId}]`
  },

  addNodeView() {
    return ({ node, editor }) => {
      const dom = document.createElement('span')
      dom.className = node.attrs.broken ? 'doc-link broken' : 'doc-link'
      dom.setAttribute('data-doc-link', node.attrs.docId)
      dom.textContent = node.attrs.docTitle || `#${node.attrs.docId}`

      // Click navigates to the linked document
      dom.addEventListener('click', () => {
        if (!node.attrs.broken) {
          window.location.hash = `/doc/${node.attrs.docId}`
        }
      })

      return { dom }
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion({
        editor: this.editor,
        ...buildLinkSuggestion(),
      }),
    ]
  },
})
