import { Editor } from '@tiptap/core'
import { buildExtensions } from './extensions.js'

/**
 * mountEditor — mounts a TipTap Editor with the shared PKD extensions onto a
 * plain DOM node. Used by the mobile WebView entry (src/editor-mobile/main.js),
 * which has no Svelte runtime. Editor.svelte builds its own Editor instance
 * directly (it needs paste handling, autosave, etc.) but shares the same
 * extensions via buildExtensions().
 *
 * @param {HTMLElement} container
 * @param {object} [options]
 * @param {string} [options.content] - initial HTML
 * @param {boolean} [options.editable=true]
 * @param {string} [options.placeholder]
 * @param {object} [options.extensions] - forwarded to buildExtensions()
 * @param {function} [options.onCreate]
 * @param {function} [options.onUpdate]
 * @param {function} [options.onTransaction]
 * @returns {Editor}
 */
export function mountEditor(container, options = {}) {
  const noop = () => {}
  return new Editor({
    element: container,
    extensions: buildExtensions(options.extensions || {}),
    content: options.content || '<p></p>',
    editable: options.editable !== false,
    editorProps: {
      attributes: {
        class: 'ProseMirror',
        'data-placeholder': options.placeholder || 'Comece a escrever…',
      },
    },
    // TipTap registers whatever is passed here as an event listener even when
    // undefined, which later crashes on emit — default to a no-op instead.
    onCreate: options.onCreate || noop,
    onUpdate: options.onUpdate || noop,
    onTransaction: options.onTransaction || noop,
  })
}
