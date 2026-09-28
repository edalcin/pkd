// Entry for the standalone editor page opened by the Android WebView
// (file:///android_asset/editor/editor.html). Built separately from the PWA —
// see vite.editor.config.js / `npm run build:editor` — so it can use
// base: './' without touching the PWA's dist/. Same TipTap schema as the PWA
// via lib/editor/extensions.js (ADR pkdMobile 0002, "Pré-requisitos no PKD").
import './editor.css'
import { NodeSelection } from '@tiptap/pm/state'
import { mountEditor } from '../lib/editor/mount-editor.js'
import { checkContentLoss } from './loss-check.js'

// window.AndroidEditor is the JS interface injected by the app. Guarded
// everywhere so the page also runs standalone in a desktop browser for testing.
const bridge = () => window.AndroidEditor

const qs = new URLSearchParams(location.search)
document.documentElement.dataset.theme = qs.get('theme') === 'dark' ? 'dark' : 'light'

const editor = mountEditor(document.getElementById('editor-root'), {
  placeholder: 'Comece a escrever…',
  extensions: {
    // The app never navigates away on tap — Link/DocLink taps show the
    // "Abrir · Editar/Remover" bubble instead (updateBubble() below).
    linkOpenOnClick: false,
    onOpenDocLink: () => {}, // ProseMirror already turns the tap into a NodeSelection; the bubble reacts to that.
    // ponytail: no [[ suggestion backend reachable from file://; the app only
    // preserves existing DocLinks, it never creates new ones (ADR: app inserts
    // only H1–H3/lists/bold/italic/link). Add a bridge search callback if that changes.
    searchDocLinks: () => [],
  },
  onCreate: () => bridge()?.onReady?.(),
  onTransaction: ({ transaction }) => {
    if (transaction.docChanged) bridge()?.onChange?.(editor.getHTML())
    reportState()
    updateBubble()
  },
})

// ---- window API for the Android bridge ----------------------------------------------------

window.pkdEditor = {
  setContent(html) {
    const original = html || '<p></p>'
    editor.setEditable(true) // a fresh setContent always gets a clean slate
    editor.commands.setContent(original, { emitUpdate: false })
    const roundTripped = editor.getHTML()
    const result = checkContentLoss(original, roundTripped)
    if (!result.ok) editor.setEditable(false)
    bridge()?.onLossCheck?.(JSON.stringify(result))
  },
  getHTML: () => editor.getHTML(),
  command: (name) => COMMANDS[name]?.(),
  setTheme(theme) {
    document.documentElement.dataset.theme = theme === 'dark' ? 'dark' : 'light'
  },
  setEditable(editable) {
    editor.setEditable(!!editable)
  },
}

const COMMANDS = {
  h1: () => editor.chain().focus().toggleHeading({ level: 1 }).run(),
  h2: () => editor.chain().focus().toggleHeading({ level: 2 }).run(),
  h3: () => editor.chain().focus().toggleHeading({ level: 3 }).run(),
  bullet: () => editor.chain().focus().toggleBulletList().run(),
  ordered: () => editor.chain().focus().toggleOrderedList().run(),
  task: () => editor.chain().focus().toggleTaskList().run(),
  bold: () => editor.chain().focus().toggleBold().run(),
  italic: () => editor.chain().focus().toggleItalic().run(),
  link: () => openLinkPrompt(editor.getAttributes('link').href || ''),
  undo: () => editor.chain().focus().undo().run(),
  redo: () => editor.chain().focus().redo().run(),
}

function reportState() {
  bridge()?.onState?.(
    JSON.stringify({
      h1: editor.isActive('heading', { level: 1 }),
      h2: editor.isActive('heading', { level: 2 }),
      h3: editor.isActive('heading', { level: 3 }),
      bullet: editor.isActive('bulletList'),
      ordered: editor.isActive('orderedList'),
      task: editor.isActive('taskList'),
      bold: editor.isActive('bold'),
      italic: editor.isActive('italic'),
      link: editor.isActive('link'),
      undo: editor.can().undo(),
      redo: editor.can().redo(),
    }),
  )
}

// ---- link bubble (Abrir · Editar · Remover) ------------------------------------------------
// Link and DocLink taps never navigate (linkOpenOnClick:false, onOpenDocLink no-op above);
// ProseMirror still turns the tap into a selection, which is all this needs.

const bubbleEl = document.getElementById('link-bubble')
const bubbleOpenBtn = document.getElementById('bubble-open')
const bubbleEditBtn = document.getElementById('bubble-edit')
const bubbleRemoveBtn = document.getElementById('bubble-remove')
let bubbleTarget = null // { kind: 'link'|'docLink', href?, docId? }

function updateBubble() {
  const { state } = editor
  const { selection } = state
  let target = null

  if (selection instanceof NodeSelection && selection.node.type.name === 'docLink') {
    target = { kind: 'docLink', docId: selection.node.attrs.docId }
  } else if (editor.isActive('link')) {
    target = { kind: 'link', href: editor.getAttributes('link').href || '' }
  }

  bubbleTarget = target
  if (!target) {
    bubbleEl.hidden = true
    // Restore the keyboard now that the selection moved to plain text.
    editor.view.dom.removeAttribute('inputmode')
    return
  }

  bubbleEditBtn.hidden = target.kind !== 'link'
  const coords = editor.view.coordsAtPos(selection.from)
  bubbleEl.hidden = false
  // While the bubble is shown, block the soft keyboard from opening.
  editor.view.dom.setAttribute('inputmode', 'none')
  const half = bubbleEl.offsetWidth / 2
  bubbleEl.style.left = `${Math.max(8, coords.left - half)}px`
  bubbleEl.style.top = `${Math.max(8, coords.top - bubbleEl.offsetHeight - 8)}px`
}

bubbleOpenBtn.onclick = () => {
  if (!bubbleTarget) return
  if (bubbleTarget.kind === 'link') bridge()?.onOpenLink?.(bubbleTarget.href)
  else bridge()?.onOpenDoc?.(String(bubbleTarget.docId))
  bubbleEl.hidden = true
}
bubbleEditBtn.onclick = async () => {
  if (!bubbleTarget || bubbleTarget.kind !== 'link') return
  editor.view.dom.removeAttribute('inputmode') // restore the keyboard for the URL prompt
  const url = await promptURL(bubbleTarget.href)
  bubbleEl.hidden = true
  if (url === null) return
  const chain = editor.chain().focus().extendMarkRange('link')
  if (url) chain.setLink({ href: url }).run()
  else chain.unsetLink().run()
}
bubbleRemoveBtn.onclick = () => {
  if (!bubbleTarget) return
  if (bubbleTarget.kind === 'link') editor.chain().focus().extendMarkRange('link').unsetLink().run()
  else editor.chain().focus().deleteSelection().run()
  bubbleEl.hidden = true
}

// ---- URL prompt (custom overlay — Android WebView has no window.prompt) -------------------

function openLinkPrompt(initial) {
  promptURL(initial).then((url) => {
    if (url === null) return
    const chain = editor.chain().focus().extendMarkRange('link')
    if (url) chain.setLink({ href: url }).run()
    else chain.unsetLink().run()
  })
}

const promptOverlay = document.getElementById('prompt-overlay')
const promptInput = document.getElementById('prompt-input')
let promptResolve = null

function promptURL(initial) {
  promptInput.value = initial || ''
  promptOverlay.hidden = false
  promptInput.focus()
  promptInput.select()
  return new Promise((resolve) => { promptResolve = resolve })
}
document.getElementById('prompt-ok').onclick = () => {
  promptOverlay.hidden = true
  promptResolve?.(promptInput.value.trim())
}
document.getElementById('prompt-cancel').onclick = () => {
  promptOverlay.hidden = true
  promptResolve?.(null)
}
promptInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') document.getElementById('prompt-ok').click()
  if (e.key === 'Escape') document.getElementById('prompt-cancel').click()
})
