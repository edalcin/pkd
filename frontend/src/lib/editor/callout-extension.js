import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

/**
 * callout-extension.js — GitHub alerts (`> [!NOTE]`, `> [!IMPORTANT]`, …).
 *
 * Markdown import/paste stores them as a plain blockquote whose first text
 * starts with the marker. Decorations only: the stored HTML keeps the literal
 * marker, so old docs render without migration, Markdown export round-trips,
 * and the mobile loss-check sees no change. Styles: .callout* in app.css and
 * editor-mobile/editor.css.
 */
const LABELS = { note: 'Note', tip: 'Tip', important: 'Important', warning: 'Warning', caution: 'Caution' }
const MARKER = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]/i

function buildDecorations(doc) {
  const decos = []
  doc.descendants((node, pos) => {
    if (node.type.name !== 'blockquote') return true
    const first = node.firstChild
    const m = first?.isTextblock && first.textContent.match(MARKER)
    if (m) {
      const type = m[1].toLowerCase()
      const start = pos + 2 // blockquote open + textblock open
      decos.push(
        Decoration.node(pos, pos + node.nodeSize, { class: `callout callout-${type}` }),
        Decoration.inline(start, start + m[0].length, { class: 'callout-title', 'data-label': LABELS[type] }),
      )
    }
    return true // nested blockquotes can be callouts too
  })
  return DecorationSet.create(doc, decos)
}

const key = new PluginKey('callout')

export const Callout = Extension.create({
  name: 'callout',
  addProseMirrorPlugins() {
    return [new Plugin({
      key,
      state: {
        init: (_, state) => buildDecorations(state.doc),
        // ponytail: full rescan per doc change; fine for note-sized docs, map+rescan changed ranges if typing lags
        apply: (tr, old) => (tr.docChanged ? buildDecorations(tr.doc) : old),
      },
      props: { decorations: state => key.getState(state) },
    })]
  },
})
