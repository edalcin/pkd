import { Node, mergeAttributes } from '@tiptap/core'
import { Suggestion } from '@tiptap/suggestion'
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

  addOptions() {
    return {
      // Called on click with the docId. Default = PWA navigation via hash.
      // The mobile WebView editor overrides this to show the link bubble instead.
      onOpen: (docId) => { window.location.hash = `/doc/${docId}` },
      // Injected [[ suggestion search, forwarded to buildLinkSuggestion.
      // undefined = default apiGet-based search (see link-suggestion.js).
      search: undefined,
    }
  },

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
    const { onOpen } = this.options
    return ({ node, editor }) => {
      const dom = document.createElement('span')
      dom.className = node.attrs.broken ? 'doc-link broken' : 'doc-link'
      dom.setAttribute('data-doc-link', node.attrs.docId)
      dom.textContent = node.attrs.docTitle || `#${node.attrs.docId}`

      // Click navigates to the linked document (or, in the mobile editor,
      // shows the link bubble — see options.onOpen).
      dom.addEventListener('click', () => {
        if (!node.attrs.broken) {
          onOpen(node.attrs.docId)
        }
      })

      return { dom }
    }
  },

  addProseMirrorPlugins() {
    return [
      Suggestion({
        editor: this.editor,
        ...buildLinkSuggestion({ search: this.options.search }),
      }),
    ]
  },
})
