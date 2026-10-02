import { StarterKit } from '@tiptap/starter-kit'
import { Table, TableRow, TableCell, TableHeader } from '@tiptap/extension-table'
import { TaskList, TaskItem } from '@tiptap/extension-list'
import { Highlight } from '@tiptap/extension-highlight'
import { TextAlign } from '@tiptap/extension-text-align'
import { ResizableImage } from './resizable-image-extension.js'
import { DocLink } from './doclink-extension.js'
import { MermaidCodeBlock } from './mermaid-code-block.js'

/**
 * extensions.js — single source of truth for the TipTap schema.
 *
 * Both the PWA (Editor.svelte) and the mobile WebView editor
 * (src/editor-mobile/main.js) build their editor from this list, so the
 * schema they read/write HTML with can never diverge (ADR pkdMobile 0002).
 *
 * @param {object} [opts]
 * @param {boolean} [opts.linkOpenOnClick=true] - PWA navigates on link click;
 *   the mobile editor sets this false and shows a bubble instead.
 * @param {(docId: string) => void} [opts.onOpenDocLink] - DocLink click handler.
 *   Default = PWA hash navigation (doclink-extension.js).
 * @param {(query: string) => Promise<Array<{id, title}>>} [opts.searchDocLinks] -
 *   [[ suggestion search. Default = apiGet('/api/search') (link-suggestion.js).
 */
export function buildExtensions(opts = {}) {
  const { linkOpenOnClick = true, onOpenDocLink, searchDocLinks } = opts
  return [
    StarterKit.configure({
      codeBlock: false,
      // v3 ships Link inside StarterKit — configured here instead of a separate extension
      link: {
        openOnClick: linkOpenOnClick,
        HTMLAttributes: { target: '_blank', rel: 'noopener noreferrer' },
        // Link no corpo rule, same as the server (internal/security/linkify.go):
        // only http:// and https:// become links. Change both together.
        shouldAutoLink: url => /^https?:\/\//i.test(url),
      },
    }),
    MermaidCodeBlock,
    ResizableImage.configure({ inline: true, allowBase64: true }),
    TaskList,
    TaskItem.configure({ nested: true }),
    Table.configure({ resizable: false }),
    TableRow,
    TableCell,
    TableHeader,
    Highlight.configure({ multicolor: true }),
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
    DocLink.configure({
      ...(onOpenDocLink ? { onOpen: onOpenDocLink } : {}),
      ...(searchDocLinks ? { search: searchDocLinks } : {}),
    }),
  ]
}
