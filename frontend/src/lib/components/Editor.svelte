<script>
  import { onDestroy } from 'svelte'
  import { Editor } from '@tiptap/core'
  import { StarterKit } from '@tiptap/starter-kit'
  import { ResizableImage } from '../editor/resizable-image-extension.js'
  import { Table, TableRow, TableCell, TableHeader } from '@tiptap/extension-table'
  import { TaskList, TaskItem } from '@tiptap/extension-list'
  import { Highlight } from '@tiptap/extension-highlight'
  import { TextAlign } from '@tiptap/extension-text-align'
  import { DocLink } from '../editor/doclink-extension.js'
  import { MermaidCodeBlock } from '../editor/mermaid-code-block.js'
  import TurndownService from 'turndown'
  import { saveDoc, loadDoc, linksRefreshSignal, docBodyRefreshedSignal, toggleLock, toggleFavorite, archiveDoc, unarchiveDoc, focusTitleForDocId, createDoc, restoreVersion, protectDoc, unprotectDoc, requestDocCode, unlockDoc, trashDoc } from '../stores/documents.js'
  import { setDocumentTags, loadTags, tags as allTags } from '../stores/tags.js'
  import { apiFetch, apiGet, apiPost, apiPut, apiPatch, apiDelete } from '../api.js'
  import IconPicker from './IconPicker.svelte'
  import VersionHistoryDialog from './VersionHistoryDialog.svelte'
  import DuplicateTitleDialog from './DuplicateTitleDialog.svelte'
  import { get } from 'svelte/store'
  import { autoSaveInterval } from '../stores/settings.js'
  import { marked } from 'marked'
  import DOMPurify from 'dompurify'
  import { DOMParser as PMParser } from '@tiptap/pm/model'

  let { docId, focusMode = false, assocPortal = null } = $props()

  let mobileTab = $state('content')
  let mobileEditMode = $state(false)
  let isMobile = $state(false)

  let doc = $state(null)
  let titleValue = $state('')
  let tagInput = $state('')
  let tagSuggestions = $state([])
  let tagSuggestionsOpen = $state(false)
  let titleInputEl = $state(null)
  let tagInputEl = $state(null)
  let tagDropdownStyle = $state('')
  let docTags = $state([])
  let relatedLinks = $state([])
  let attachments = $state([])
  let docUrls = $state([])
  let assocYear = $state(null)
  let assocMonth = $state(null)
  let assocDay = $state(null)
  let children = $state([])
  let ancestors = $state([])
  let assocPaneEl = $state(null)

  let urlInput = $state('')
  let urlTitleInput = $state('')
  let urlAdding = $state(false)
  let editingUrlId = $state(null)
  let editUrlValue = $state('')
  let editUrlTitle = $state('')
  let iconPickerOpen = $state(false)

  // Map tag name → tag object (color, text_color), derived from the global tags store
  const tagByName = $derived(
    Object.fromEntries(($allTags || []).map(t => [t.name, t]))
  )

  let highlightColor = $state('#fef08a') // default yellow

  // Link insertion — URL popover
  let linkOpen = $state(false)
  let linkHref = $state('')
  let linkText = $state('')
  let extLinkInputEl = $state(null)
  let linkTitleInputEl = $state(null)

  $effect(() => {
    if (linkOpen && extLinkInputEl) {
      // Pre-fill with existing link href if cursor is inside one
      const existing = editorInstance?.getAttributes('link').href || ''
      linkHref = existing
      // Pre-fill text with selection (only if no existing link)
      if (!existing) {
        const { from, to } = editorInstance?.state.selection ?? {}
        linkText = from != null && to != null && from !== to
          ? editorInstance.state.doc.textBetween(from, to)
          : ''
      }
      setTimeout(() => (linkTitleInputEl ?? extLinkInputEl)?.focus(), 30)
    }
  })

  function toggleLink(e) {
    e.preventDefault()
    linkOpen = !linkOpen
    if (!linkOpen) { linkHref = ''; linkText = '' }
  }

  function insertLink() {
    const href = linkHref.trim()
    if (!href) { editorInstance?.chain().focus().unsetLink().run(); linkOpen = false; return }
    const chain = editorInstance?.chain().focus()
    // If there's selected text, wrap it; otherwise insert the text (or href) as a link node
    const { from, to } = editorInstance?.state.selection ?? {}
    const hasSelection = from != null && to != null && from !== to
    if (hasSelection) {
      chain.setLink({ href }).run()
    } else {
      const label = linkText.trim() || href
      chain.insertContent(`<a href="${href}" target="_blank" rel="noopener noreferrer">${label}</a>`).run()
    }
    linkOpen = false
    linkHref = ''
    linkText = ''
    scheduleAutoSave()
  }

  function onLinkBlur(e) {
    const next = e.relatedTarget
    if (next && next.closest?.('.tb-panel')) return
    setTimeout(() => { linkOpen = false; linkHref = ''; linkText = '' }, 150)
  }

  function clearLink(e) {
    e.preventDefault()
    editorInstance?.chain().focus().unsetLink().run()
    linkOpen = false
    scheduleAutoSave()
  }

  // Image insertion — URL popover + upload
  let imgUrlOpen = $state(false)
  let imgUrlValue = $state('')
  let imgUrlInputEl = $state(null)
  let imgUploading = $state(false)
  let importingImgs = $state(false)
  let importImgsResult = $state(null) // {imported, failed} — shown briefly after import
  let uploadImgInputEl = $state(null)
  let importMdInputEl = $state(null)

  function triggerImageUpload(e) {
    e.preventDefault()
    uploadImgInputEl?.click()
  }

  function triggerMarkdownImport(e) {
    e.preventDefault() // keeps the editor selection while the file dialog is open
    importMdInputEl?.click()
  }

  $effect(() => {
    if (imgUrlOpen && imgUrlInputEl) {
      setTimeout(() => imgUrlInputEl?.focus(), 30)
    }
  })

  function toggleImgUrl(e) {
    e.preventDefault()
    imgUrlOpen = !imgUrlOpen
    if (!imgUrlOpen) imgUrlValue = ''
  }

  function insertImageByURL() {
    const url = imgUrlValue.trim()
    if (url) editorInstance?.chain().focus().setImage({ src: url }).run()
    imgUrlOpen = false
    imgUrlValue = ''
  }

  function onImgUrlBlur() {
    // Delay so that clicking "Inserir" fires submit before closing
    setTimeout(() => { imgUrlOpen = false; imgUrlValue = '' }, 150)
  }

  // Image upload — inserts into editor body AND registers as attachment
  async function handleImageUploadInline(e) {
    const file = e.target.files?.[0]
    if (!file || !doc) return
    imgUploading = true
    try {
      const fd = new FormData()
      fd.append('file', file)
      const res = await apiFetch(`/api/documents/${doc.id}/attachments?inline=1`, { method: 'POST', body: fd })
      if (!res.ok) return
      const att = await res.json()
      if (att?.url) {
        editorInstance?.chain().focus().setImage({ src: att.url }).run()
        scheduleAutoSave()
        // Refresh attachment list so it appears in the footer too
        try { attachments = await apiGet(`/api/documents/${doc.id}/attachments`) } catch {}
      }
    } finally {
      imgUploading = false
      e.target.value = ''
    }
  }

  // $derived: list of unique external image srcs in the current editor state.
  // Re-computed each time editorTick changes (i.e. on every TipTap transaction),
  // which guarantees immediate update after the import dispatch rewrites the srcs.
  let externalImgSrcs = $derived.by(() => {
    if (editorTick < 0 || !editorInstance) return [] // reads editorTick as reactive dep
    const srcs = new Set()
    editorInstance.state.doc.descendants(node => {
      if (node.type.name === 'image') {
        const src = node.attrs?.src || ''
        if ((src.startsWith('http://') || src.startsWith('https://')) && !src.includes('/api/attachments/')) {
          srcs.add(src)
        }
      }
    })
    return [...srcs]
  })

  // Import all external images: fetch via backend, rewrite srcs in one transaction.
  // Uses a snapshot of externalImgSrcs taken at call time to avoid re-fetching
  // images that were already rewritten by a previous import in the same session.
  async function importExternalImages(e) {
    e?.preventDefault()
    if (!doc || importingImgs) return
    const srcs = externalImgSrcs // snapshot — avoids re-fetching already-imported
    if (srcs.length === 0) return
    importingImgs = true
    try {
      const mapping = {}
      for (const src of srcs) {
        try {
          const res = await apiFetch(`/api/documents/${doc.id}/attachments/from-url`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: src }),
          })
          if (res.ok) {
            const att = await res.json()
            if (att?.url) mapping[src] = att.url
          }
        } catch { /* skip failed image, keep original src */ }
      }
      const failed = srcs.length - Object.keys(mapping).length
      if (Object.keys(mapping).length === 0) {
        importImgsResult = { imported: 0, failed }
        setTimeout(() => { importImgsResult = null }, 5000)
        return
      }
      // Rewrite all affected image nodes in one ProseMirror transaction.
      // dispatch() triggers onTransaction → editorTick++ → externalImgSrcs recomputes
      // → empty list (if all mapped) → button disappears immediately.
      const { tr, doc: pmDoc } = editorInstance.state
      pmDoc.descendants((node, pos) => {
        if (node.type.name === 'image') {
          const newSrc = mapping[node.attrs?.src]
          if (newSrc) {
            tr.setNodeMarkup(pos, undefined, { ...node.attrs, src: newSrc })
          }
        }
      })
      editorInstance.view.dispatch(tr)
      importImgsResult = { imported: Object.keys(mapping).length, failed }
      setTimeout(() => { importImgsResult = null }, 5000)
      await performSave()
      try { attachments = await apiGet(`/api/documents/${doc.id}/attachments`) } catch {}
    } finally {
      importingImgs = false
    }
  }

  // Attachment preview modal
  let previewAtt = $state(null)
  let markdownContent = $state('')
  let markdownLoading = $state(false)

  function isMarkdown(mime, name) {
    if (mime === 'text/markdown' || mime === 'text/x-markdown') return true
    const ext = (name || '').split('.').pop().toLowerCase()
    return ext === 'md' || ext === 'markdown'
  }

  $effect(() => {
    const att = previewAtt
    if (!att || !isMarkdown(att.mime_type, att.original_name)) {
      markdownContent = ''
      return
    }
    markdownLoading = true
    fetch(`/api/attachments/${att.id}`)
      .then(r => r.text())
      .then(text => { markdownContent = DOMPurify.sanitize(marked.parse(text)) })
      .catch(() => { markdownContent = '<p><em>(Erro ao carregar conteúdo)</em></p>' })
      .finally(() => { markdownLoading = false })
  })

  function openPreview(att) {
    previewAtt = att
  }
  function closePreview() {
    previewAtt = null
  }
  function previewType(mime, name) {
    if (!mime && !name) return 'download'
    if (mime?.startsWith('image/')) return 'image'
    if (mime === 'application/pdf') return 'pdf'
    if (mime?.startsWith('audio/')) return 'audio'
    if (mime?.startsWith('video/')) return 'video'
    if (isMarkdown(mime, name)) return 'markdown'
    return 'download'
  }

  // Link search state
  let linkQuery = $state('')
  let linkResults = $state([])
  let linkSearchOpen = $state(false)
  let linkSearchTimer = null
  let dropdownStyle = $state('')
  let linkHrefInputEl = $state(null)
  let saving = $state(false)
  let linkCopied = $state(false)
  let saveError = $state('')
  let conflictData = $state(null)
  let titleConflictData = $state(null)
  let loading = $state(true)
  let historyOpen = $state(false)

  // TipTap Editor instance (not reactive — managed manually)
  let editorInstance = null
  let autoSaveTimer = null
  // True when a focus-mode popup is open for this doc — autosave blocked in origin window
  let focusModeActive = $state(false)

  // editorReady signals that the editor has been mounted (triggers toolbar render).
  // editorTick increments on each transaction to keep toolbar active-states fresh.
  let editorReady = $state(false)
  let editorTick = $state(0)

  // Encrypted-document unlock panel state
  let unlockChallengeId = $state('')
  let unlockCode = $state('')
  let unlockErr = $state('')
  let unlockSending = $state(false)
  let useBackupUnlock = $state(false)
  let unlockBackupCode = $state('')

  function isActive(type, attrs) {
    return editorInstance?.isActive(type, attrs) ?? false
  }

  function fmt(cmd) {
    // Execute a TipTap chain command and keep focus in the editor.
    // Each toolbar action is a function that receives the chain builder.
    cmd(editorInstance.chain().focus()).run()
  }

  onDestroy(() => {
    clearTimeout(autoSaveTimer)
    editorInstance?.destroy()
  })

  // Coordinate autosave disable/enable with focus-mode popup via BroadcastChannel
  $effect(() => {
    const ch = new BroadcastChannel(`focus-doc-${docId}`)
    if (focusMode) {
      // Focus window: signal origin to suspend autosave
      ch.postMessage('opened')
    } else {
      ch.onmessage = (e) => {
        if (e.data === 'opened') { focusModeActive = true; clearTimeout(autoSaveTimer) }
        if (e.data === 'closed') focusModeActive = false
      }
    }
    return () => {
      if (focusMode) ch.postMessage('closed')
      ch.close()
    }
  })

  $effect(() => {
    const mq = window.matchMedia('(max-width: 640px)')
    isMobile = mq.matches
    const handler = (e) => { isMobile = e.matches }
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  })

  // Teleport assoc-pane into the App-level sidebar portal on desktop.
  // The returned cleanup removes the element from the portal when this
  // Editor instance is destroyed (e.g. {#key route.id} swap), preventing
  // stale panes from accumulating in the sidebar.
  $effect(() => {
    const portal = assocPortal
    const el = assocPaneEl
    if (portal && el) {
      portal.appendChild(el)
      return () => el.remove()
    }
  })

  async function loadDocument() {
    const targetId = Number(docId)
    mobileEditMode = false
    mobileTab = 'content'
    loading = true
    ancestors = []
    try {
      const loadedDoc = await loadDoc(targetId)
      if (Number(docId) !== targetId) return  // navigation changed mid-flight, abort
      doc = loadedDoc
      titleValue = doc.title
      docTags = doc.tags || []
      if (doc.assoc_year != null) {
        assocYear = doc.assoc_year
        assocMonth = doc.assoc_month ?? null
        assocDay = doc.assoc_day ?? null
      } else {
        const today = new Date()
        assocYear = today.getFullYear()
        assocMonth = today.getMonth() + 1
        assocDay = today.getDate()
      }
      loadTags()
      await loadLinks(targetId)
      if (Number(docId) !== targetId) return
      try {
        const atts = await apiGet(`/api/documents/${targetId}/attachments`)
        if (Number(docId) !== targetId) return
        attachments = atts || []
      } catch {
        attachments = doc?.attachment_ids?.map(id => ({ id })) || []
      }
      try {
        const urls = await apiGet(`/api/documents/${targetId}/urls`)
        if (Number(docId) !== targetId) return
        docUrls = urls || []
      } catch {
        docUrls = []
      }
      try {
        const ch = await apiGet(`/api/documents/${targetId}/children`) || []
        if (Number(docId) !== targetId) return
        children = ch
      } catch {
        children = []
      }
      try {
        const ancs = await apiGet(`/api/documents/${targetId}/ancestors`)
        if (Number(docId) !== targetId) return
        ancestors = ancs || []
      } catch {
        ancestors = []
      }
    } finally {
      // Only clear loading if we're still the current document.
      // A newer loadDocument() in flight will clear it when it finishes.
      if (Number(docId) === targetId) {
        loading = false
        if (get(focusTitleForDocId) === targetId) {
          focusTitleForDocId.set(null)
          setTimeout(() => {
            titleInputEl?.focus()
            if (titleValue.endsWith(' - ')) {
              const len = titleValue.length
              titleInputEl?.setSelectionRange(len, len)
            } else {
              titleInputEl?.select()
            }
          }, 50)
        }
      }
    }
  }

  async function loadLinks(id) {
    const targetId = id ?? Number(docId)
    try {
      const resp = await apiGet(`/api/documents/${targetId}/links`)
      relatedLinks = resp.related || []
    } catch {
      relatedLinks = []
    }
  }

  // Reload when docId prop changes
  $effect(() => {
    if (docId) {
      editorInstance?.destroy()
      editorInstance = null
      loadDocument()
    }
  })

  // Reload links when a relationship is created from the sidebar
  $effect(() => {
    if ($linksRefreshSignal && doc) loadLinks(doc.id)
  })

  // Reload editor content when the document body is updated externally (e.g. admin image import).
  // Uses setContent to update in place — no editor re-mount, no docId change needed.
  $effect(() => {
    const sig = $docBodyRefreshedSignal
    if (sig === Number(docId) && doc && editorInstance) {
      loadDoc(sig).then(freshDoc => {
        if (!freshDoc || !editorInstance) return
        editorInstance.commands.setContent(freshDoc.body_html || '', { emitUpdate: false })
        doc = freshDoc
        titleValue = freshDoc.title
        try { apiGet(`/api/documents/${sig}/attachments`).then(a => { attachments = a || [] }) } catch {}
      })
    }
  })

  // Auto-save: interval configured by user (0 = disabled); skip for locked docs or focus popup open
  function scheduleAutoSave() {
    const interval = get(autoSaveInterval)
    if (interval === 0) return
    if (doc?.locked) return
    if (focusModeActive) return
    clearTimeout(autoSaveTimer)
    autoSaveTimer = setTimeout(performSave, interval)
  }

  function handleIconSelect(icon) {
    doc = { ...doc, icon }
    iconPickerOpen = false
    scheduleAutoSave()
  }

  async function handleToggleFavorite() {
    const updated = await toggleFavorite(doc.id)
    doc = updated
  }

  async function handleToggleLock() {
    const updated = await toggleLock(doc.id)
    doc = updated
  }

  async function handleToggleArchive() {
    const updated = doc.archived ? await unarchiveDoc(doc.id) : await archiveDoc(doc.id)
    doc = updated
  }

  async function handleDeleteDoc() {
    if (!confirm(`Mover "${doc.title || 'Sem título'}" para a lixeira?`)) return
    await trashDoc(doc.id)
    window.location.hash = '/'
  }

  async function handleToggleProtect() {
    try {
      if (!doc.encrypted) {
        const updated = await protectDoc(doc.id)
        doc = updated // updated.body_html is plaintext, updated.encrypted === true — editor content unchanged
      } else {
        const updated = await unprotectDoc(doc.id)
        doc = updated
        editorInstance?.commands.setContent(updated.body_html || '', { emitUpdate: false })
      }
      saveError = ''
    } catch (e) {
      if (e?.status === 503) saveError = 'E-mail 2FA não configurado.'
      else if (e?.status === 403) saveError = 'É preciso desbloquear o documento antes de desproteger.'
      else saveError = 'Falha ao alterar proteção do documento.'
    }
  }

  async function handleRequestUnlockCode() {
    unlockErr = ''
    unlockSending = true
    try {
      const res = await requestDocCode(doc.id)
      if (res && res.challenge_id) {
        unlockChallengeId = res.challenge_id
      } else {
        // 204: already unlocked in this session — just reload
        await loadDocument()
      }
    } catch (e) {
      unlockErr = e?.status === 503 ? 'E-mail 2FA não configurado.' : 'Falha ao enviar código.'
    } finally {
      unlockSending = false
    }
  }

  async function handleUnlockSubmit() {
    unlockErr = ''
    try {
      const updated = await unlockDoc(doc.id, unlockChallengeId, unlockCode)
      doc = { ...updated, encrypted_locked: false }
      unlockChallengeId = ''
      unlockCode = ''
      // The tiptap-editor div only mounts once !doc.encrypted_locked, and
      // mountEditor() reads doc.body_html at mount time — so the fresh mount
      // already picks up the decrypted content. This covers the (unexpected)
      // case where an editor instance already exists.
      if (editorInstance) editorInstance.commands.setContent(updated.body_html || '', { emitUpdate: false })
    } catch (e) {
      unlockErr = 'Código inválido ou expirado.'
      unlockCode = ''
    }
  }

  async function handleUnlockBackup() {
    unlockErr = ''
    try {
      const updated = await unlockDoc(doc.id, '', unlockBackupCode.trim())
      doc = { ...updated, encrypted_locked: false }
      unlockBackupCode = ''
      useBackupUnlock = false
      if (editorInstance) editorInstance.commands.setContent(updated.body_html || '', { emitUpdate: false })
    } catch {
      unlockErr = 'Código de backup inválido.'
      unlockBackupCode = ''
    }
  }

  async function handleCreateSubDoc() {
    const sub = await createDoc(doc.id, `${doc.title} - `)
    window.location.hash = `/doc/${sub.id}`
  }

  $effect(() => {
    if (editorReady && editorInstance) {
      editorInstance.setEditable(!doc?.locked && !(isMobile && !focusMode && !mobileEditMode))
      if (doc?.locked) { clearTimeout(autoSaveTimer); autoSaveTimer = null }
    }
  })

  function handleCopyLink() {
    const url = `${window.location.origin}/#/doc/${doc.id}`
    navigator.clipboard.writeText(url).then(() => {
      linkCopied = true
      setTimeout(() => { linkCopied = false }, 1500)
    })
  }

  async function performSave() {
    if (!doc || !editorInstance) return
    if (doc.locked) return
    if (saving) { scheduleAutoSave(); return }
    if (conflictData) return
    const html = editorInstance.getHTML()
    const text = editorInstance.getText()
    saving = true
    saveError = ''
    try {
      const result = await saveDoc(doc.id, {
        version: doc.version,
        title: titleValue,
        body_html: html,
        body_text: text,
        icon: doc.icon || '',
      })
      if (result?._conflict) {
        if (result.conflict_type === 'title') {
          titleConflictData = result
        } else {
          conflictData = result
        }
        return
      }
      doc = result
    } catch {
      saveError = 'Erro ao salvar'
    } finally {
      saving = false
    }
  }

  async function exportMarkdown() {
    if (!editorInstance || !doc) return
    const td = new TurndownService({ headingStyle: 'atx', codeBlockStyle: 'fenced' })
    td.addRule('docLink', {
      filter: node => node.nodeName === 'SPAN' && node.hasAttribute('data-doc-link'),
      replacement: (content) => `[[${content}]]`,
    })
    td.addRule('taskItem', {
      filter: node => node.nodeName === 'LI' && node.getAttribute('data-type') === 'taskItem',
      replacement: (content, node) => {
        const prefix = `- [${node.getAttribute('data-checked') === 'true' ? 'x' : ' '}] `
        const isParagraph = /\n$/.test(content)
        content = content.replace(/^\n+/, '').replace(/\n+$/, '') + (isParagraph ? '\n' : '')
        content = content.replace(/\n/gm, '\n' + ' '.repeat(prefix.length))
        return prefix + content + (node.nextSibling ? '\n' : '')
      },
    })
    const md = td.turndown(editorInstance.getHTML())
    const filename = (doc.title || 'documento').replace(/[/\\?%*:|"<>]/g, '-') + '.md'
    if ('showSaveFilePicker' in window) {
      try {
        const handle = await window.showSaveFilePicker({
          suggestedName: filename,
          types: [{ description: 'Markdown', accept: { 'text/markdown': ['.md'] } }],
        })
        const writable = await handle.createWritable()
        await writable.write(md)
        await writable.close()
        return
      } catch (e) {
        if (e.name === 'AbortError') return
        // fall through to blob download on other errors
      }
    }
    // ponytail: fallback for browsers without File System Access API
    const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
    URL.revokeObjectURL(url)
  }

  async function handleConflictOverwrite() {
    if (!doc) return
    doc = { ...doc, version: conflictData.stored_version }
    conflictData = null
    await performSave()
  }

  async function handleConflictReload() {
    conflictData = null
    await loadDocument()
  }

  function handleTitleConflictNavigate() {
    if (!titleConflictData) return
    const id = titleConflictData.existing_doc.id
    titleConflictData = null
    window.location.hash = `/doc/${id}`
  }

  function handleTitleConflictClose() {
    if (!doc) return
    titleValue = doc.title
    titleConflictData = null
  }

  // Tags + autocomplete
  function onTagInput() {
    const q = tagInput.trim().toLowerCase().replace(/^#/, '')
    if (!q) { tagSuggestions = []; tagSuggestionsOpen = false; return }
    tagSuggestions = ($allTags || [])
      .map(t => t.name)
      .filter(n => n.includes(q) && !docTags.includes(n))
      .slice(0, 8)
    tagSuggestionsOpen = tagSuggestions.length > 0
    if (tagSuggestionsOpen && tagInputEl) {
      const r = tagInputEl.getBoundingClientRect()
      tagDropdownStyle = `top:${r.bottom + 4}px;left:${r.left}px`
    }
  }

  function closeTagSuggestions() {
    tagSuggestionsOpen = false
    tagSuggestions = []
  }

  async function commitTag(name) {
    const normalized = name.trim().toLowerCase().replace(/^#/, '').replace(/\s+/g, '-')
    if (normalized && !docTags.includes(normalized)) {
      docTags = [...docTags, normalized]
      await setDocumentTags(doc.id, docTags)
    }
    tagInput = ''
    closeTagSuggestions()
  }

  async function addTag(e) {
    if (e.key === 'Escape') { closeTagSuggestions(); return }
    if (e.key !== 'Enter' && e.key !== ',') return
    e.preventDefault()
    await commitTag(tagInput)
  }

  async function removeTag(name) {
    docTags = docTags.filter(t => t !== name)
    await setDocumentTags(doc.id, docTags)
  }

  // ── Associated date ───────────────────────────────────────────────────────

  function daysInMonth(year, month) {
    if (!year || !month) return 31
    return new Date(year, month, 0).getDate()
  }

  $effect(() => {
    if (!assocMonth) assocDay = null
  })

  async function saveAssocDate() {
    if (!doc) return
    try {
      const updated = await apiPatch(`/api/documents/${doc.id}/associated-date`, {
        year: assocYear ?? null,
        month: assocMonth ?? null,
        day: assocDay ?? null,
      })
      if (updated) {
        doc = { ...doc, assoc_year: updated.assoc_year, assoc_month: updated.assoc_month, assoc_day: updated.assoc_day }
      }
    } catch { /* silent — user can retry */ }
  }

  function clearAssocDate() {
    assocYear = null
    assocMonth = null
    assocDay = null
    saveAssocDate()
  }

  function formatAssocDate() {
    if (!assocYear) return ''
    const months = ['Janeiro','Fevereiro','Março','Abril','Maio','Junho','Julho','Agosto','Setembro','Outubro','Novembro','Dezembro']
    if (!assocMonth) return `${assocYear}`
    if (!assocDay) return `${months[assocMonth - 1]}/${assocYear}`
    return `${String(assocDay).padStart(2,'0')}/${String(assocMonth).padStart(2,'0')}/${assocYear}`
  }

  // ── Documentos relacionados ────────────────────────────────────────────────

  function onLinkInput() {
    clearTimeout(linkSearchTimer)
    if (linkQuery.trim().length < 1) { linkResults = []; linkSearchOpen = false; return }
    linkSearchTimer = setTimeout(async () => {
      try {
        const hits = await apiGet(`/api/search?q=${encodeURIComponent(linkQuery)}`)
        const linked = new Set(relatedLinks.map(l => l.related_id))
        linkResults = (hits || []).filter(h => h.id !== doc.id && !linked.has(h.id)).slice(0, 8)
        if (linkResults.length > 0) {
          // Position dropdown below the input using fixed coords
          if (linkHrefInputEl) {
            const r = linkHrefInputEl.getBoundingClientRect()
            dropdownStyle = `top:${r.bottom + 4}px;left:${r.left}px`
          }
          linkSearchOpen = true
        } else {
          linkSearchOpen = false
        }
      } catch { linkResults = [] }
    }, 200)
  }

  async function addLink(targetId, targetTitle) {
    linkQuery = ''
    linkResults = []
    linkSearchOpen = false
    try {
      await apiPost(`/api/documents/${doc.id}/links`, { target_id: targetId })
      await loadLinks()
    } catch (err) {
      if (!err.message?.includes('already exists')) {
        saveError = 'Erro ao relacionar nota'
      }
    }
  }

  async function removeLink(linkId) {
    await apiDelete(`/api/documents/${doc.id}/links/${linkId}`)
    relatedLinks = relatedLinks.filter(l => l.id !== linkId)
  }

  function closeLinkSearch() {
    linkSearchOpen = false
    linkResults = []
  }

  // ── Attachment helpers ────────────────────────────────────────────────────
  function isImage(mime) {
    return !!mime?.startsWith('image/')
  }

  function fileIcon(mime) {
    if (!mime) return '📎'
    if (mime === 'application/pdf') return '📄'
    if (mime.startsWith('audio/')) return '🎵'
    if (mime.startsWith('video/')) return '🎬'
    if (mime.includes('word') || mime.includes('officedocument.wordprocessing')) return '📝'
    if (mime.includes('excel') || mime.includes('spreadsheet') || mime === 'text/csv') return '📊'
    if (mime.includes('powerpoint') || mime.includes('presentation')) return '📑'
    if (mime.startsWith('text/')) return '📃'
    if (mime.includes('zip') || mime.includes('archive') || mime.includes('compressed')) return '🗜️'
    return '📎'
  }

  // File attachment upload
  async function handleFileUpload(e) {
    const file = e.target.files?.[0]
    if (!file) return
    const fd = new FormData()
    fd.append('file', file)
    const res = await apiFetch(`/api/documents/${doc.id}/attachments`, {
      method: 'POST',
      body: fd,
    })
    if (res.ok) {
      const att = await res.json()
      attachments = [...attachments, att]
    }
  }

  async function deleteAttachment(id) {
    if (!confirm('Remover anexo?')) return
    await apiDelete(`/api/attachments/${id}`)
    attachments = attachments.filter(a => a.id !== id)
  }

  async function addURL() {
    const url = urlInput.trim()
    if (!url) return
    urlAdding = true
    try {
      const u = await apiPost(`/api/documents/${doc.id}/urls`, { url, title: urlTitleInput.trim() })
      docUrls = [...docUrls, u]
      urlInput = ''
      urlTitleInput = ''
    } catch {
      saveError = 'Erro ao adicionar link'
    } finally {
      urlAdding = false
    }
  }

  async function deleteURL(id) {
    await apiDelete(`/api/documents/${doc.id}/urls/${id}`)
    docUrls = docUrls.filter(u => u.id !== id)
  }

  function startEditUrl(u) {
    editingUrlId = u.id
    editUrlValue = u.url
    editUrlTitle = u.title
  }

  function cancelEditUrl() {
    editingUrlId = null
    editUrlValue = ''
    editUrlTitle = ''
  }

  async function saveEditUrl() {
    const url = editUrlValue.trim()
    if (!url) return
    try {
      const updated = await apiPut(`/api/documents/${doc.id}/urls/${editingUrlId}`, { url, title: editUrlTitle.trim() })
      docUrls = docUrls.map(u => u.id === editingUrlId ? updated : u)
      cancelEditUrl()
    } catch {
      saveError = 'Erro ao editar link'
    }
  }

  // True when plain-text clipboard content carries Markdown block/inline syntax.
  function looksLikeMarkdown(text) {
    if (!text) return false
    return (
      /^#{1,6}\s/m.test(text) ||                 // ATX heading
      /^\s*([-*+])\s+\S/m.test(text) ||          // unordered list item
      /^\s*\d+\.\s+\S/m.test(text) ||            // ordered list item
      /^\s*>\s/m.test(text) ||                   // blockquote
      /^\s*(```|~~~)/m.test(text) ||             // fenced code
      /^\s*([-*_])\1{2,}\s*$/m.test(text) ||     // thematic break --- *** ___
      /\*\*[^*\n]+\*\*/.test(text) ||            // **bold**
      /__[^_\n]+__/.test(text) ||                // __bold__
      /\[[^\]]+\]\([^)\s]+\)/.test(text) ||      // [text](url)
      /(^|\n)\s*\|.+\|\s*(\n|$)/.test(text)      // | table | row |
    )
  }

  // True when the clipboard's text/html already carries real structure/formatting,
  // meaning TipTap's default paste should render it (don't override from plain text).
  function htmlHasRichStructure(html) {
    if (!html) return false
    return /<(h[1-6]|ul|ol|li|blockquote|pre|code|table|thead|tbody|tr|td|th|img|a|strong|em|b|i|u|mark|figure)\b/i.test(html)
  }

  // Collapse extra blank lines between table rows so GFM table parsing succeeds
  // even when the source (AI chat, etc.) inserts a blank line after each row.
  // Works with both \n and \r\n line endings.
  function normalizeMarkdownTables(text) {
    return text.replace(/(\|[^\r\n]*\r?\n)(?:\r?\n)+(\s*\|)/g, '$1$2')
  }

  // marked renders GFM checklists as plain <li><input type=checkbox>…</li> with no
  // data-type markers, so TipTap's TaskList/TaskItem parseHTML rules (which require
  // ul[data-type="taskList"] / li[data-type="taskItem"]) never match on paste. Retag
  // any <ul> whose items are ALL checkboxes so pasted checklists become real, checkable
  // taskList nodes instead of losing their checkbox state. Mixed lists are left alone.
  function normalizeMarkdownTaskLists(container) {
    container.querySelectorAll('ul').forEach(ul => {
      const items = Array.from(ul.children).filter(c => c.tagName === 'LI')
      if (!items.length) return
      const checkboxItems = items.filter(li => li.firstElementChild?.matches('input[type="checkbox"]'))
      if (checkboxItems.length !== items.length) return
      ul.setAttribute('data-type', 'taskList')
      items.forEach(li => {
        const checkbox = li.firstElementChild
        li.setAttribute('data-type', 'taskItem')
        li.setAttribute('data-checked', checkbox.checked ? 'true' : 'false')
        checkbox.remove()
      })
    })
  }

  // Markdown text → ProseMirror slice, inserted at the current selection.
  // PMParser.fromSchema + replaceSelection (instead of insertContent) because
  // TipTap's parseSlice path can silently drop table nodes.
  function insertMarkdownIntoView(view, text) {
    const rendered = DOMPurify.sanitize(marked.parse(normalizeMarkdownTables(text)))
    const tempEl = document.createElement('div')
    tempEl.innerHTML = rendered
    normalizeMarkdownTaskLists(tempEl)
    const slice = PMParser.fromSchema(view.state.schema).parseSlice(tempEl)
    view.dispatch(view.state.tr.replaceSelection(slice))
  }

  async function handleMarkdownImportInline(e) {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-importing the same file
    if (!file || !editorInstance || doc?.locked) return
    try {
      const text = await file.text()
      if (!text.trim()) return
      if (!editorInstance.isFocused) editorInstance.commands.focus('end')
      insertMarkdownIntoView(editorInstance.view, text)
      saveError = ''
    } catch {
      saveError = 'Erro ao importar Markdown'
    }
  }

  // Combined paste handler: images → upload & insert; Markdown → convert & insert; else → default.
  function handlePaste(view, event) {
    // Image branch (unchanged behavior)
    const items = [...(event.clipboardData?.items || [])]
    const imageItem = items.find(i => i.type.startsWith('image/'))
    if (imageItem) {
      const file = imageItem.getAsFile()
      if (file) {
        const fd = new FormData()
        fd.append('file', file)
        apiFetch(`/api/documents/${doc.id}/attachments`, { method: 'POST', body: fd })
          .then(r => r.json())
          .then(att => {
            if (att?.url) {
              view.dispatch(view.state.tr.replaceSelectionWith(
                view.state.schema.nodes.image.create({ src: att.url })
              ))
            }
          })
        return true
      }
    }

    // Markdown branch: convert when plain text looks like Markdown and HTML has no real structure
    const text = event.clipboardData?.getData('text/plain') || ''
    const html = event.clipboardData?.getData('text/html') || ''
    if (text && looksLikeMarkdown(text) && !htmlHasRichStructure(html)) {
      insertMarkdownIntoView(view, text)
      return true
    }

    // Fallthrough: TipTap default handles plain text and genuine rich HTML
    return false
  }

  // Ctrl+S saves the document from anywhere in the editor area
  function handleKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault()
      performSave()
    }
    if (e.key === 'Escape' && previewAtt) {
      closePreview()
    }
    if (e.key === 'Escape' && focusMode) {
      window.close()
    }
  }

  // Svelte action: mounts the TipTap editor on the DOM node
  function mountEditor(node) {
    let mounted = false
    editorInstance = new Editor({
      element: node,
      extensions: [
        StarterKit.configure({
          codeBlock: false,
          // v3 ships Link inside StarterKit — configured here instead of a separate extension
          link: {
            openOnClick: true,
            HTMLAttributes: { target: '_blank', rel: 'noopener noreferrer' },
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
        DocLink,
      ],
      content: doc?.body_html || '',
      editorProps: {
        handlePaste: handlePaste,
        attributes: {
          class: 'ProseMirror',
          'data-placeholder': 'Comece a escrever…',
        },
      },
      onCreate: () => { mounted = true },
      onUpdate: () => { if (mounted) scheduleAutoSave() },
      onTransaction: () => { editorTick++; editorReady = true },
    })
    editorReady = true

    return {
      destroy() {
        clearTimeout(autoSaveTimer)
        editorInstance?.destroy()
        editorInstance = null
        editorReady = false
      },
    }
  }
</script>

{#if loading}
  <div class="editor-area">
    <div class="empty-state"><div class="spinner"></div></div>
  </div>
{:else if doc}
  <div class="editor-area" onkeydown={handleKeydown} role="region" aria-label="Editor de documento" tabindex="-1">
    <div class="content-pane" class:mobile-pane-hidden={isMobile && mobileTab !== 'content'}>
    <!-- Title -->
    <div class="doc-header">
      <div class="title-row">
        <div class="doc-icon-wrap">
          <button
            class="doc-icon-btn"
            onclick={() => iconPickerOpen = !iconPickerOpen}
            title="Definir ícone do documento"
            aria-label="Ícone"
            disabled={doc.locked}
          >
            <i class="bx {doc.icon || 'bx-file-blank'}" style={doc.icon ? '' : 'opacity:.25'}></i>
          </button>
          {#if iconPickerOpen}
            <IconPicker
              value={doc.icon || ''}
              onSelect={handleIconSelect}
              onClose={() => iconPickerOpen = false}
            />
          {/if}
        </div>
        <input
          class="doc-title"
          type="text"
          bind:this={titleInputEl}
          bind:value={titleValue}
          oninput={scheduleAutoSave}
          placeholder="Título do documento"
          aria-label="Título"
          disabled={doc.locked}
        />
        <button
          class="subdoc-btn"
          onclick={handleCreateSubDoc}
          title="Criar sub-documento"
          aria-label="Criar sub-documento"
        ><i class="bx bxs-file-plus"></i></button>
        <button
          class="copy-link-btn"
          onclick={handleCopyLink}
          title="Copiar link do documento"
          aria-label="Copiar link"
        ><i class="bx {linkCopied ? 'bx-check' : 'bx-link'}"></i></button>
        <button
          class="fav-btn {doc.is_favorite ? 'is-favorite' : ''}"
          onclick={handleToggleFavorite}
          title={doc.is_favorite ? 'Remover dos favoritos' : 'Favoritar documento'}
          aria-label={doc.is_favorite ? 'Remover dos favoritos' : 'Favoritar documento'}
        ><i class="bx {doc.is_favorite ? 'bxs-star' : 'bx-star'}"></i></button>
        <button
          class="lock-btn {doc.locked ? 'is-locked' : ''}"
          onclick={handleToggleLock}
          title={doc.locked ? 'Destrancar documento' : 'Trancar documento'}
          aria-label={doc.locked ? 'Destrancar' : 'Trancar'}
        ><i class="bx {doc.locked ? 'bx-lock' : 'bx-lock-open'}"></i></button>
        <button
          class="protect-btn {doc.encrypted ? 'is-protected' : ''}"
          onclick={handleToggleProtect}
          title={doc.encrypted ? 'Desproteger documento' : 'Proteger (encriptar) documento'}
          aria-label={doc.encrypted ? 'Desproteger' : 'Proteger'}
        ><i class="bx {doc.encrypted ? 'bxs-shield' : 'bx-shield'}"></i></button>
        <button
          class="archive-btn {doc.archived ? 'is-archived' : ''}"
          onclick={handleToggleArchive}
          title={doc.archived ? 'Desarquivar documento' : 'Arquivar documento'}
          aria-label={doc.archived ? 'Desarquivar' : 'Arquivar'}
        ><i class="bx bx-archive"></i></button>
        <button
          class="delete-btn"
          onclick={handleDeleteDoc}
          title="Mover para a lixeira"
          aria-label="Mover para a lixeira"
        ><i class="bx bx-trash"></i></button>
        <button
          class="hist-btn"
          onclick={() => historyOpen = true}
          title="Histórico de versões"
          aria-label="Histórico de versões"
        ><i class="bx bx-history"></i></button>
        <button
          class="save-icon-btn {saving ? 'saving' : ''}"
          onclick={performSave}
          disabled={saving || doc.locked || doc.archived}
          title="Salvar (Ctrl+S)"
          aria-label="Salvar"
        ><i class="bx {saving ? 'bx-loader-alt bx-spin' : 'bx-save'}"></i></button>
      </div>
      {#if ancestors.length > 0}
        <nav class="doc-breadcrumb" aria-label="Localização do documento">
          {#each ancestors as anc}
            <button
              class="breadcrumb-item"
              onclick={() => window.location.hash = `/doc/${anc.id}`}
              title={anc.title}
            >{#if anc.icon}<i class="bx {anc.icon}"></i>{/if}{anc.title}</button>
            <span class="breadcrumb-sep" aria-hidden="true">›</span>
          {/each}
          <span class="breadcrumb-current">{#if doc.icon}<i class="bx {doc.icon}"></i>{/if}{doc.title || 'Sem título'}</span>
        </nav>
      {/if}
      <div class="doc-meta">
        {#each docTags as tag}
          {@const t = tagByName[tag]}
          {@const c = t?.color || ''}
          <span
            class="tag-chip active"
            style={c ? `background:${c}; border-color:${c}; color:${t?.text_color || '#fff'}` : ''}
          >
            #{tag}
            <button class="tag-remove" onclick={() => removeTag(tag)} aria-label="Remover tag" disabled={doc.locked}>×</button>
          </span>
        {/each}
        <div class="tag-input-wrap">
          <input
            class="tag-input"
            type="text"
            bind:value={tagInput}
            bind:this={tagInputEl}
            oninput={onTagInput}
            onkeydown={addTag}
            onblur={() => setTimeout(closeTagSuggestions, 150)}
            placeholder="+ tag"
            aria-label="Adicionar tag"
            autocomplete="off"
            disabled={doc.locked}
          />
          {#if tagSuggestionsOpen}
            <div class="tag-dropdown" role="listbox" style={tagDropdownStyle}>
              {#each tagSuggestions as s}
                <div
                  class="tag-option"
                  role="option"
                  aria-selected="false"
                  tabindex="0"
                  onmousedown={e => { e.preventDefault(); commitTag(s) }}
                  onkeydown={e => e.key === 'Enter' && commitTag(s)}
                ># {s}</div>
              {/each}
            </div>
          {/if}
        </div>
        <span class="save-status">
          {#if saving}⏳{:else if saveError}❌ {saveError}{:else}✓{/if}
        </span>
      </div>
    </div>

    <!-- Archived banner -->
    {#if doc.archived && doc.locked}
      <div class="archived-banner">
        <i class="bx bx-archive"></i>
        <span>Este documento está arquivado e é somente leitura.</span>
        <button class="archived-unarchive-btn" onclick={handleToggleArchive}>Desarquivar</button>
      </div>
    {/if}

    {#if !doc.encrypted_locked}
    <!-- Formatting toolbar — always visible when a document is open.
         {#key editorTick} forces re-evaluation of isActive() on every
         TipTap transaction (selection change, content edit). -->
    {#key editorTick}
    <div class="toolbar {doc.locked ? 'toolbar-locked' : ''} {isMobile && !mobileEditMode ? 'mobile-toolbar-hidden' : ''}" role="toolbar" aria-label="Formatação">
        {#if isMobile && mobileEditMode}
          <button class="tb-btn tb-done-mobile" onclick={() => { performSave(); mobileEditMode = false }} title="Salvar e sair">✓ Pronto</button>
          <div class="tb-sep" role="separator"></div>
        {/if}
        <!-- Headings -->
        <button class="tb-btn {isActive('heading', {level:1}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleHeading({level:1})) }} title="Título 1" aria-pressed={isActive('heading',{level:1})}>H1</button>
        <button class="tb-btn {isActive('heading', {level:2}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleHeading({level:2})) }} title="Título 2" aria-pressed={isActive('heading',{level:2})}>H2</button>
        <button class="tb-btn {isActive('heading', {level:3}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleHeading({level:3})) }} title="Título 3" aria-pressed={isActive('heading',{level:3})}>H3</button>

        <div class="tb-sep" role="separator"></div>

        <!-- Inline styles -->
        <button class="tb-btn {isActive('bold') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleBold()) }} title="Negrito (Ctrl+B)" aria-pressed={isActive('bold')}><strong>B</strong></button>
        <button class="tb-btn {isActive('italic') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleItalic()) }} title="Itálico (Ctrl+I)" aria-pressed={isActive('italic')}><em>I</em></button>
        <button class="tb-btn {isActive('strike') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleStrike()) }} title="Tachado" aria-pressed={isActive('strike')}><s>S</s></button>
        <button class="tb-btn {isActive('code') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleCode()) }} title="Código inline" aria-pressed={isActive('code')}><code>`</code></button>

        <div class="tb-sep" role="separator"></div>

        <!-- Blocks -->
        <button class="tb-btn {isActive('bulletList') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleBulletList()) }} title="Lista" aria-pressed={isActive('bulletList')}>☰</button>
        <button class="tb-btn {isActive('orderedList') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleOrderedList()) }} title="Lista numerada" aria-pressed={isActive('orderedList')}>1.</button>
        <button class="tb-btn {isActive('taskList') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleTaskList()) }} title="Checklist" aria-pressed={isActive('taskList')}>☑</button>
        <button class="tb-btn {isActive('blockquote') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleBlockquote()) }} title="Citação" aria-pressed={isActive('blockquote')}>"</button>
        <button class="tb-btn {isActive('codeBlock') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleCodeBlock()) }} title="Bloco de código" aria-pressed={isActive('codeBlock')}>&lt;/&gt;</button>

        <div class="tb-sep" role="separator"></div>

        <!-- Text alignment -->
        <button class="tb-btn {isActive({textAlign:'left'}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.setTextAlign('left')) }} title="Alinhar à esquerda">⬅</button>
        <button class="tb-btn {isActive({textAlign:'center'}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.setTextAlign('center')) }} title="Centralizar">⬛</button>
        <button class="tb-btn {isActive({textAlign:'right'}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.setTextAlign('right')) }} title="Alinhar à direita">➡</button>
        <button class="tb-btn {isActive({textAlign:'justify'}) ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.setTextAlign('justify')) }} title="Justificar">≡</button>

        <div class="tb-sep" role="separator"></div>

        <!-- Highlight -->
        <button class="tb-btn tb-highlight {isActive('highlight') ? 'active' : ''}"
          onmousedown={e => { e.preventDefault(); fmt(c => c.toggleHighlight({ color: highlightColor })) }}
          title="Realçar texto">
          <span style="background:{highlightColor};padding:0 3px;border-radius:2px">H</span>
        </button>
        <input type="color" class="tb-color-input" bind:value={highlightColor} title="Cor do realce" />

        <div class="tb-sep" role="separator"></div>

        <!-- Link -->
        <button class="tb-btn {isActive('link') ? 'active' : ''}" onmousedown={toggleLink} title="Inserir / editar link">🔗</button>

        <!-- Image: URL + upload -->
        <button class="tb-btn {imgUrlOpen ? 'active' : ''}" onmousedown={toggleImgUrl} title="Inserir imagem por URL">🖼</button>
        <button class="tb-btn" onmousedown={triggerImageUpload} title={imgUploading ? 'Enviando…' : 'Fazer upload de imagem'}>{imgUploading ? '⏳' : '📤'}</button>
        {#if externalImgSrcs.length > 0 || importingImgs || importImgsResult}
          <button class="tb-btn tb-btn-import-imgs" onmousedown={importExternalImages}
            disabled={importingImgs || externalImgSrcs.length === 0}
            title={importingImgs ? 'Importando…' : externalImgSrcs.length > 0 ? `Importar ${externalImgSrcs.length} imagem(ns) externa(s) como anexo` : 'Importação concluída'}>
            {importingImgs ? '⏳' : externalImgSrcs.length > 0 ? '🌐⬇' : '✓'}
          </button>
          {#if importImgsResult}
            <span class="tb-import-result {importImgsResult.failed > 0 && importImgsResult.imported === 0 ? 'tb-import-error' : ''}">
              {importImgsResult.imported > 0 ? `${importImgsResult.imported} importada(s)` : ''}
              {importImgsResult.failed > 0 ? `${importImgsResult.failed} falhou` : ''}
            </span>
          {/if}
        {/if}

        <!-- Table -->
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.insertTable({ rows: 3, cols: 3, withHeaderRow: true })) }} title="Inserir tabela">⊞</button>
        {#if isActive('table')}
          <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.addRowAfter()) }} title="Nova linha abaixo">+↓</button>
          <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.addColumnAfter()) }} title="Nova coluna à direita">+→</button>
          <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.deleteRow()) }} title="Remover linha">-↓</button>
          <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.deleteColumn()) }} title="Remover coluna">-→</button>
          <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.deleteTable()) }} title="Remover tabela" style="color:var(--danger,#ef4444)">✕⊞</button>
        {/if}

        <div class="tb-sep" role="separator"></div>

        <!-- Utility -->
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.setHorizontalRule()) }} title="Linha horizontal">—</button>
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.undo()) }} title="Desfazer (Ctrl+Z)">↩</button>
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); fmt(c => c.redo()) }} title="Refazer (Ctrl+Y)">↪</button>

        <div class="tb-spacer"></div>

        <!-- Import / Export Markdown -->
        <button class="tb-btn" onmousedown={triggerMarkdownImport} disabled={doc.locked} title="Importar Markdown (.md) na posição do cursor">⬆ .md</button>
        <button class="tb-btn" onclick={exportMarkdown} title="Exportar como Markdown (.md)">⬇ .md</button>

        <div class="tb-sep" role="separator"></div>

        <!-- Focus mode -->
        <button class="tb-btn {focusMode ? 'active' : ''}"
          onclick={() => focusMode ? window.close() : (focusModeActive = true, clearTimeout(autoSaveTimer), window.open(`#/focus/${docId}`, '_blank', 'width=960,height=720,resizable=yes'))}
          title={focusMode ? 'Fechar janela (Esc)' : 'Abrir em modo foco'}>
          {focusMode ? '✕' : '⛶'}
        </button>
      </div>
    {/key}

    <!-- File input lives outside {#key editorTick} so it survives editor transactions
         (opening the file dialog causes a blur transaction that would otherwise destroy it) -->
    <input bind:this={uploadImgInputEl} type="file" accept="image/*" class="sr-only" onchange={handleImageUploadInline} disabled={imgUploading} />
    <input bind:this={importMdInputEl} type="file" accept=".md,.markdown,.txt,text/markdown,text/plain" class="sr-only" onchange={handleMarkdownImportInline} />

    <!-- Inline panels that must live outside the overflow:auto toolbar -->
    {#if linkOpen}
      <div class="tb-panel">
        <span class="tb-panel-label">🔗 Link</span>
        {#if !editorInstance?.getAttributes('link').href}
          <input
            bind:this={linkTitleInputEl}
            bind:value={linkText}
            class="tb-panel-input"
            type="text"
            placeholder="Título (opcional)"
            style="width:160px"
            onblur={onLinkBlur}
            onkeydown={e => { if (e.key === 'Enter') { e.preventDefault(); insertLink() } else if (e.key === 'Escape') { linkOpen = false; linkHref = ''; linkText = '' } }}
          />
        {/if}
        <input
          bind:this={extLinkInputEl}
          bind:value={linkHref}
          class="tb-panel-input"
          type="url"
          placeholder="https://…"
          onblur={onLinkBlur}
          onkeydown={e => { if (e.key === 'Enter') { e.preventDefault(); insertLink() } else if (e.key === 'Escape') { linkOpen = false; linkHref = ''; linkText = '' } }}
        />
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); insertLink() }}>Inserir</button>
        {#if isActive('link')}
          <button class="tb-btn" onmousedown={clearLink} style="color:var(--danger,#ef4444)" title="Remover link">✕ Remover</button>
        {/if}
      </div>
    {/if}
    {#if imgUrlOpen}
      <div class="tb-panel">
        <span class="tb-panel-label">🖼 Imagem por URL</span>
        <input
          bind:this={imgUrlInputEl}
          bind:value={imgUrlValue}
          class="tb-panel-input"
          type="url"
          placeholder="https://…"
          onblur={onImgUrlBlur}
          onkeydown={e => { if (e.key === 'Enter') { e.preventDefault(); insertImageByURL() } else if (e.key === 'Escape') { imgUrlOpen = false; imgUrlValue = '' } }}
        />
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); insertImageByURL() }}>Inserir</button>
      </div>
    {/if}

    <!-- TipTap editor -->
    <div class="tiptap-editor" use:mountEditor></div>
    {:else}
      <div class="encrypted-locked-panel">
        <i class="bx bxs-shield-x" style="font-size:2rem"></i>
        <p>Este documento está protegido. Envie um código por e-mail para abri-lo.</p>
        {#if useBackupUnlock}
          <input type="text" bind:value={unlockBackupCode} placeholder="XXXXX-XXXXX" autocomplete="off" />
          <button class="btn btn-primary" onclick={handleUnlockBackup} disabled={!unlockBackupCode.trim()}>Abrir</button>
          <button type="button" style="background:none;border:none;color:var(--text);text-decoration:underline;cursor:pointer;font-size:.85rem" onclick={() => { useBackupUnlock = false; unlockErr = '' }}>
            Usar código por e-mail
          </button>
        {:else if !unlockChallengeId}
          <button class="btn btn-primary" onclick={handleRequestUnlockCode} disabled={unlockSending}>
            {unlockSending ? 'Enviando…' : 'Enviar código por e-mail'}
          </button>
          <button type="button" style="background:none;border:none;color:var(--text);text-decoration:underline;cursor:pointer;font-size:.85rem" onclick={() => { useBackupUnlock = true; unlockErr = '' }}>
            Usar código de backup
          </button>
        {:else}
          <input type="text" maxlength="6" inputmode="numeric" pattern="[0-9]*" bind:value={unlockCode} placeholder="000000" />
          <button class="btn btn-primary" onclick={handleUnlockSubmit} disabled={unlockCode.length !== 6}>Abrir</button>
        {/if}
        {#if unlockErr}<p class="error-msg" role="alert">{unlockErr}</p>{/if}
      </div>
    {/if}

    </div><!-- /content-pane -->

    <!-- ── Área de associações ───────────────────────────────────── -->
    {#if !focusMode}
      {#if isMobile && !mobileEditMode && !doc.locked}
        <button class="mobile-fab-edit" onclick={() => mobileEditMode = true} aria-label="Editar documento" title="Editar">✏️</button>
      {/if}

      <div class="assoc-pane" bind:this={assocPaneEl} class:mobile-pane-hidden={isMobile && mobileTab !== 'assoc'}>
        {#if isMobile}
          <div class="mobile-assoc-header">
            <i class="bx {doc.icon || 'bx-file-blank'}"></i>
            <span class="mobile-assoc-title">{doc.title || 'Sem título'}</span>
          </div>
        {/if}

        <!-- ── Sub-documentos (movido para o painel direito) ─── -->
        {#if children.length > 0}
          <div class="children-area">
            <div class="children-header">
              <span class="children-label">Sub-documentos</span>
            </div>
            <div class="children-grid">
              {#each children as child}
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <div
                  class="child-card"
                  onclick={() => { window.location.hash = `/doc/${child.id}` }}
                  role="button"
                  tabindex="0"
                  onkeydown={e => e.key === 'Enter' && (window.location.hash = `/doc/${child.id}`)}
                >
                  <div class="child-card-title">
                    <i class="bx {child.icon || 'bx-file-blank'} child-card-icon"></i>
                    <span class="child-card-name">{child.title || 'Sem título'}</span>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        {/if}

    <div class="assoc-area">
      <div class="assoc-divider">
        <span class="assoc-divider-label">Associações</span>
      </div>

      <div class="assoc-grid">

        <!-- Coluna 1: Documentos relacionados -->
        <section class="assoc-col">
          <h4 class="assoc-col-title">📄 Documentos relacionados</h4>

          <!-- busca para adicionar -->
          <div class="link-search-wrap">
            <input
              class="assoc-search-input"
              type="text"
              bind:value={linkQuery}
              bind:this={linkHrefInputEl}
              oninput={onLinkInput}
              onblur={() => setTimeout(closeLinkSearch, 150)}
              placeholder="Buscar documento para relacionar…"
              aria-label="Relacionar nota"
              autocomplete="off"
            />
            {#if linkSearchOpen}
              <div class="link-dropdown" role="listbox" style={dropdownStyle}>
                {#each linkResults as hit}
                  <div
                    class="link-option"
                    role="option"
                    aria-selected="false"
                    tabindex="0"
                    onmousedown={e => { e.preventDefault(); addLink(hit.id, hit.title) }}
                    onkeydown={e => e.key === 'Enter' && addLink(hit.id, hit.title)}
                  >📄 {hit.title}</div>
                {/each}
              </div>
            {/if}
          </div>

          <!-- related docs (undirected) -->
          {#each relatedLinks as link}
            <div class="assoc-item">
              <span
                class="assoc-item-label link-title {link.related_trashed ? 'broken' : ''}"
                role="button" tabindex={link.related_trashed ? -1 : 0}
                onclick={() => !link.related_trashed && (window.location.hash = `/doc/${link.related_id}`)}
                onkeydown={e => e.key === 'Enter' && !link.related_trashed && (window.location.hash = `/doc/${link.related_id}`)}
              >{link.related_title}{#if link.related_trashed}<span class="broken-badge">excluído</span>{/if}</span>
              <button class="row-btn row-btn-del" onmousedown={e => { e.preventDefault(); removeLink(link.id) }} title="Remover">×</button>
            </div>
          {/each}

          {#if relatedLinks.length === 0}
            <p class="assoc-empty">Nenhum documento relacionado</p>
          {/if}
        </section>

        <!-- Coluna 2: Arquivos -->
        <section class="assoc-col">
          <h4 class="assoc-col-title">📎 Arquivos</h4>

          {#if attachments.length === 0}
            <p class="assoc-empty">Nenhum arquivo anexado</p>
          {:else}
            <div class="att-grid">
              {#each attachments as att}
                <div class="att-card">
                  <button
                    class="att-preview"
                    title={att.original_name || `Anexo #${att.id}`}
                    onclick={() => openPreview(att)}
                  >
                    {#if isImage(att.mime_type)}
                      <img
                        src="/api/attachments/{att.id}"
                        alt={att.original_name}
                        class="att-thumb"
                        loading="lazy"
                      />
                    {:else}
                      <span class="att-icon">{fileIcon(att.mime_type)}</span>
                    {/if}
                  </button>
                  <div class="att-card-footer">
                    <span class="att-name" title={att.original_name}>
                      {att.original_name || `Anexo #${att.id}`}
                    </span>
                    <button
                      class="att-del-btn"
                      onclick={() => deleteAttachment(att.id)}
                      title="Remover"
                    >×</button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}

          <label class="assoc-add-btn">
            + Anexar arquivo
            <input type="file" class="hidden-input" onchange={handleFileUpload} />
          </label>
        </section>

        <!-- Coluna 3: Links externos -->
        <section class="assoc-col">
          <h4 class="assoc-col-title">🔗 Links externos</h4>

          {#each docUrls as u}
            {#if editingUrlId === u.id}
              <div class="assoc-item url-edit-mode">
                <input
                  class="url-title-input"
                  type="text"
                  bind:value={editUrlTitle}
                  placeholder="Título (opcional)"
                  aria-label="Título do link"
                  onkeydown={e => { if (e.key === 'Enter') saveEditUrl(); if (e.key === 'Escape') cancelEditUrl() }}
                />
                <input
                  class="url-input"
                  type="url"
                  bind:value={editUrlValue}
                  placeholder="https://…"
                  aria-label="URL"
                  onkeydown={e => { if (e.key === 'Enter') saveEditUrl(); if (e.key === 'Escape') cancelEditUrl() }}
                />
                <div class="url-edit-actions">
                  <button class="url-edit-btn url-edit-btn-save" onclick={saveEditUrl} disabled={!editUrlValue.trim()}>✓ Salvar</button>
                  <button class="url-edit-btn url-edit-btn-cancel" onclick={cancelEditUrl}>Cancelar</button>
                </div>
              </div>
            {:else}
              <div class="assoc-item">
                <a href={u.url} target="_blank" rel="noopener noreferrer" class="assoc-item-label url-link" title={u.url}>
                  {u.title || u.url}
                </a>
                <button class="row-btn row-btn-edit" onclick={() => startEditUrl(u)} title="Editar">✎</button>
                <button class="row-btn row-btn-del" onclick={() => deleteURL(u.id)} title="Remover">×</button>
              </div>
            {/if}
          {/each}

          {#if docUrls.length === 0}
            <p class="assoc-empty">Nenhum link externo</p>
          {/if}

          <div class="url-add-row">
            <input
              class="url-title-input"
              type="text"
              bind:value={urlTitleInput}
              placeholder="Título (opcional)"
              aria-label="Título do link"
              onkeydown={e => e.key === 'Enter' && addURL()}
            />
            <input
              class="url-input"
              type="url"
              bind:value={urlInput}
              placeholder="https://…"
              aria-label="URL"
              onkeydown={e => e.key === 'Enter' && addURL()}
            />
            <button class="assoc-add-btn" onclick={addURL} disabled={urlAdding || !urlInput.trim()}>
              + Adicionar
            </button>
          </div>
        </section>

        <!-- Coluna 4: Data associada -->
        <section class="assoc-col">
          <h4 class="assoc-col-title">📅 Data associada</h4>

          {#if doc}
            <p class="assoc-date-created">
              <span class="assoc-date-label">Criação:</span>
              {new Date(doc.created_at).toLocaleString('pt-BR')}
            </p>
          {/if}

          <div class="assoc-date-row">
            <select
              class="assoc-date-select"
              bind:value={assocDay}
              onchange={saveAssocDate}
              disabled={!assocMonth}
              aria-label="Dia"
            >
              <option value={null}>Dia</option>
              {#each Array.from({length: daysInMonth(assocYear, assocMonth)}, (_, i) => i + 1) as d}
                <option value={d}>{d}</option>
              {/each}
            </select>

            <select
              class="assoc-date-select"
              bind:value={assocMonth}
              onchange={saveAssocDate}
              aria-label="Mês"
            >
              <option value={null}>Mês</option>
              {#each ['Janeiro','Fevereiro','Março','Abril','Maio','Junho','Julho','Agosto','Setembro','Outubro','Novembro','Dezembro'] as m, i}
                <option value={i + 1}>{m}</option>
              {/each}
            </select>

            <select
              class="assoc-date-select assoc-date-year"
              bind:value={assocYear}
              onchange={saveAssocDate}
              aria-label="Ano"
            >
              <option value={null}>Ano</option>
              {#each Array.from({length: new Date().getFullYear() - 1900 + 11}, (_, i) => new Date().getFullYear() + 10 - i) as y}
                <option value={y}>{y}</option>
              {/each}
            </select>
          </div>

          {#if assocYear || assocMonth || assocDay}
            <p class="assoc-date-display">{formatAssocDate()}</p>
          {/if}

          <button class="assoc-clear-date-btn" onclick={clearAssocDate}>
            Limpar data
          </button>
        </section>

      </div>
    </div><!-- /assoc-area -->
      </div><!-- /assoc-pane -->

      {#if isMobile}
      <nav class="mobile-tab-bar" aria-label="Navegação de seções">
        <button class:active={mobileTab === 'content'} onclick={() => mobileTab = 'content'} aria-pressed={mobileTab === 'content'}>
          <span class="tab-icon">📄</span>
          <span class="tab-label">Conteúdo</span>
        </button>
        <button class:active={mobileTab === 'assoc'} onclick={() => { mobileTab = 'assoc'; mobileEditMode = false }} aria-pressed={mobileTab === 'assoc'}>
          <span class="tab-icon">🔗</span>
          <span class="tab-label">Associações</span>
        </button>
      </nav>
      {/if}
    {/if}
  </div>

  <!-- Attachment preview modal -->
  {#if previewAtt}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="preview-backdrop" onclick={closePreview}>
      <div class="preview-box" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Visualizar arquivo">

        <div class="preview-header">
          <span class="preview-title">{previewAtt.original_name || `Anexo #${previewAtt.id}`}</span>
          <div class="preview-actions">
            <a href="/api/attachments/{previewAtt.id}" download={previewAtt.original_name} class="preview-dl-btn" title="Baixar">⬇ Baixar</a>
            <button class="preview-close" onclick={closePreview} aria-label="Fechar">✕</button>
          </div>
        </div>

        <div class="preview-body">
          {#if previewType(previewAtt.mime_type, previewAtt.original_name) === 'image'}
            <img src="/api/attachments/{previewAtt.id}" alt={previewAtt.original_name} class="preview-img" />

          {:else if previewType(previewAtt.mime_type, previewAtt.original_name) === 'pdf'}
            <embed
              src="/api/attachments/{previewAtt.id}"
              type="application/pdf"
              class="preview-pdf"
            />

          {:else if previewType(previewAtt.mime_type, previewAtt.original_name) === 'audio'}
            <div class="preview-audio-wrap">
              <span class="preview-big-icon">🎵</span>
              <p class="preview-file-name">{previewAtt.original_name}</p>
              <!-- svelte-ignore a11y_media_has_caption -->
              <audio controls src="/api/attachments/{previewAtt.id}" class="preview-audio"></audio>
            </div>

          {:else if previewType(previewAtt.mime_type, previewAtt.original_name) === 'video'}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video controls src="/api/attachments/{previewAtt.id}" class="preview-video"></video>

          {:else if previewType(previewAtt.mime_type, previewAtt.original_name) === 'markdown'}
            <div class="preview-markdown">
              {#if markdownLoading}
                <p class="muted">Carregando…</p>
              {:else}
                {@html markdownContent}
              {/if}
            </div>

          {:else}
            <div class="preview-download-wrap">
              <span class="preview-big-icon">{fileIcon(previewAtt.mime_type)}</span>
              <p class="preview-file-name">{previewAtt.original_name}</p>
              <p class="preview-file-size">{previewAtt.size_bytes ? (previewAtt.size_bytes / 1024).toFixed(0) + ' KB' : ''}</p>
              <a href="/api/attachments/{previewAtt.id}" download={previewAtt.original_name} class="btn btn-primary preview-dl-large">
                ⬇ Fazer download
              </a>
            </div>
          {/if}
        </div>

      </div>
    </div>
  {/if}

  <!-- Version history dialog -->
  {#if historyOpen && doc?.id}
    <VersionHistoryDialog
      docId={doc.id}
      currentVersion={doc.version}
      onClose={() => historyOpen = false}
      onRestored={(updated) => {
        doc = updated
        editorInstance?.commands.setContent(updated.body_html, { emitUpdate: false })
        historyOpen = false
      }}
    />
  {/if}

  <!-- Version conflict dialog -->
  {#if conflictData}
    <div class="modal-backdrop">
      <div class="modal">
        <h2>Conflito de versão</h2>
        <p>Este documento foi alterado por outra aba. O que deseja fazer?</p>
        <div class="modal-actions">
          <button class="btn btn-ghost" onclick={handleConflictReload}>Descartar e recarregar</button>
          <button class="btn btn-primary" onclick={handleConflictOverwrite}>Sobrescrever com minha versão</button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Title conflict dialog -->
  {#if titleConflictData}
    <DuplicateTitleDialog
      conflict={titleConflictData}
      onNavigate={handleTitleConflictNavigate}
      onClose={handleTitleConflictClose}
    />
  {/if}
{:else}
  <div class="editor-area empty-state">
    <span class="emoji">📝</span>
    <p>Selecione um documento ou crie um novo</p>
  </div>
{/if}

<style>
  /* ── Archived banner ──────────────────────────── */
  .archived-banner {
    display: flex;
    align-items: center;
    gap: .6rem;
    padding: .45rem .75rem;
    margin-bottom: .5rem;
    background: color-mix(in srgb, var(--text-muted) 10%, transparent);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: .82rem;
    color: var(--text-muted);
  }

  .archived-banner i {
    font-size: 1rem;
    flex-shrink: 0;
  }

  .archived-banner span {
    flex: 1;
  }

  .archived-unarchive-btn {
    flex-shrink: 0;
    padding: .2rem .6rem;
    font-size: .8rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--accent);
    cursor: pointer;
    transition: background .12s, border-color .12s;
  }

  .archived-unarchive-btn:hover {
    background: var(--bg-hover);
    border-color: var(--accent);
  }

  /* ── Encrypted document unlock panel ────────────── */
  .encrypted-locked-panel {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: .75rem;
    padding: 3rem 1.5rem;
    color: var(--text-muted);
    text-align: center;
  }

  .encrypted-locked-panel i {
    color: var(--accent);
  }

  .encrypted-locked-panel input {
    width: 140px;
    padding: .5rem;
    font-size: 1.3rem;
    letter-spacing: .3rem;
    text-align: center;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
  }

  .encrypted-locked-panel .error-msg {
    color: var(--danger);
    font-size: .875rem;
  }

  /* ── Formatting toolbar ─────────────────────────── */
  .toolbar {
    display: flex;
    align-items: center;
    flex-wrap: nowrap;
    overflow-x: auto;
    gap: 2px;
    padding: .25rem .25rem;
    margin-bottom: .5rem;
    background: var(--bg-secondary, var(--bg-sidebar));
    border: 1px solid var(--border);
    border-radius: 6px;
    scrollbar-width: none; /* Firefox */
  }
  .toolbar::-webkit-scrollbar { display: none; } /* Chrome/Safari */
  .toolbar-locked { opacity: .35; pointer-events: none; }

  .tb-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 28px;
    height: 28px;
    padding: 0 6px;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text);
    font-size: .85rem;
    cursor: pointer;
    transition: background .12s;
    white-space: nowrap;
  }

  .tb-btn:hover {
    background: var(--bg-hover);
  }

  .tb-btn.active {
    background: var(--accent);
    color: #fff;
  }

  .tb-btn-import-imgs {
    color: var(--accent);
  }

  .tb-import-result {
    font-size: .8em;
    color: var(--accent);
    white-space: nowrap;
    align-self: center;
  }
  .tb-import-result.tb-import-error {
    color: var(--danger, #ef4444);
  }

  .tb-btn:disabled {
    opacity: .5;
    cursor: not-allowed;
  }

  .tb-sep {
    width: 1px;
    height: 20px;
    background: var(--border);
    margin: 0 3px;
    flex-shrink: 0;
  }

  .tb-spacer {
    flex: 1;
  }

  /* Hidden file input (triggered programmatically) */
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0,0,0,0);
    white-space: nowrap;
  }

  /* Inline panel that appears below the toolbar (outside overflow container) */
  .tb-panel {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    margin-bottom: 4px;
    background: var(--bg-secondary, var(--bg-sidebar));
    border: 1px solid var(--border);
    border-radius: 6px;
    flex-wrap: wrap;
  }

  .tb-panel-label {
    font-size: .8rem;
    color: var(--text-muted, #6b7280);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .tb-panel-input {
    flex: 1;
    min-width: 180px;
    padding: 3px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
    font-size: .85rem;
    background: var(--bg);
    color: var(--text);
  }

  /* Make links in the editor body visually obvious and cursor-correct */
  :global(.ProseMirror a) {
    color: var(--accent, #3b82f6);
    text-decoration: underline;
    cursor: pointer;
  }

  .tb-color-input {
    width: 24px;
    height: 22px;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 3px;
    cursor: pointer;
    background: none;
    flex-shrink: 0;
  }

  .doc-header { margin-bottom: .75rem; }

  .title-row {
    display: flex;
    align-items: center;
    gap: .5rem;
    margin-bottom: .5rem;
  }

  .doc-breadcrumb {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: .15rem;
    margin-bottom: .35rem;
    font-size: .75rem;
    color: var(--text-muted, #888);
    min-height: 1.2rem;
  }
  .breadcrumb-item {
    background: none;
    border: none;
    padding: .1rem .2rem;
    border-radius: 3px;
    cursor: pointer;
    color: var(--text-muted, #888);
    font-size: .75rem;
    white-space: nowrap;
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: color .15s, background .15s;
  }
  .breadcrumb-item:hover { color: var(--accent); background: var(--bg-hover); }
  .breadcrumb-item i { margin-right: .2rem; font-size: .8rem; }
  .breadcrumb-sep { color: var(--text-muted, #888); opacity: .5; user-select: none; }
  .breadcrumb-current {
    color: var(--text-muted, #888);
    font-size: .75rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 200px;
    opacity: .7;
  }
  .breadcrumb-current i { margin-right: .2rem; font-size: .8rem; }


  .doc-icon-wrap {
    position: relative;
    flex-shrink: 0;
  }

  .doc-icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    border-radius: 8px;
    font-size: 1.75rem;
    cursor: pointer;
    background: none;
    border: 1px solid transparent;
    color: var(--text);
    transition: background .15s, border-color .15s;
    flex-shrink: 0;
  }
  .doc-icon-btn:hover { background: var(--bg-hover); border-color: var(--border); }
  .doc-icon-btn:disabled { opacity: .35; cursor: default; }

  .lock-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .lock-btn:hover { background: var(--bg-hover); }
  .lock-btn.is-locked { color: var(--accent); }

  .protect-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .protect-btn:hover { background: var(--bg-hover); }
  .protect-btn.is-protected { color: var(--accent); }

  .fav-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text-muted);
    transition: color .15s, background .15s;
  }
  .fav-btn:hover { background: var(--bg-hover); color: #f5c518; }
  .fav-btn.is-favorite { color: #f5c518; }

  .archive-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .archive-btn:hover:not(:disabled) { background: var(--bg-hover); }
  .archive-btn.is-archived { color: var(--accent); }
  .archive-btn:disabled { opacity: .4; cursor: default; }

  .delete-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .delete-btn:hover { background: var(--danger); color: #fff; }

  .hist-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .hist-btn:hover:not(:disabled) { background: var(--bg-hover); }
  .hist-btn:disabled { opacity: .4; cursor: default; }

  .save-icon-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .save-icon-btn:hover:not(:disabled) { background: var(--bg-hover); color: var(--accent); }
  .save-icon-btn:disabled { opacity: .35; cursor: default; }
  .save-icon-btn.saving { opacity: .7; }

  .copy-link-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .copy-link-btn:hover { background: var(--bg-hover); color: var(--accent); }

  .subdoc-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 1.1rem;
    color: var(--text);
    transition: color .15s, background .15s;
  }
  .subdoc-btn:hover { background: var(--bg-hover); color: var(--accent); }

  .tag-input-wrap {
    position: relative;
    display: inline-block;
  }

  .tag-input {
    border: none;
    border-bottom: 1px solid var(--border);
    border-radius: 0;
    padding: .1rem .25rem;
    font-size: .8rem;
    width: 80px;
    background: transparent;
  }

  .tag-dropdown {
    position: fixed;
    min-width: 160px;
    max-width: 280px;
    background: var(--bg-panel);
    border: 1px solid var(--border);
    border-radius: 5px;
    z-index: 1000;
    box-shadow: 0 4px 12px rgba(0,0,0,.25);
    max-height: 200px;
    overflow-y: auto;
  }

  .tag-option {
    padding: .35rem .65rem;
    font-size: .85rem;
    cursor: pointer;
    white-space: nowrap;
    color: var(--accent);
  }

  .tag-option:hover { background: var(--bg-hover); }

  .tag-remove {
    font-size: .75rem;
    color: inherit;
    opacity: .7;
    cursor: pointer;
    padding: 0 2px;
  }

  .save-status {
    margin-left: auto;
    font-size: .75rem;
    color: var(--text-muted);
  }

  .broken-badge {
    font-size: .7rem;
    background: var(--bg-hover);
    padding: .1rem .3rem;
    border-radius: 3px;
    margin-left: .25rem;
  }

  /* ── Sub-documentos ─────────────────────────────────────── */
  .children-area {
    margin-top: 1.5rem;
    margin-bottom: .5rem;
  }

  .children-header {
    display: flex;
    align-items: center;
    gap: .5rem;
    margin-bottom: .75rem;
  }

  .children-label {
    font-size: .75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: .06em;
  }

  .children-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: .75rem;
  }

  .child-card {
    background: var(--bg-secondary, var(--bg-sidebar));
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: .5rem .75rem;
    cursor: pointer;
    transition: border-color .15s, box-shadow .15s;
    overflow: hidden;
  }

  .child-card:hover {
    border-color: var(--accent);
    box-shadow: 0 2px 8px rgba(0,0,0,.12);
  }

  .child-card-title {
    display: flex;
    align-items: center;
    gap: .4rem;
    margin-bottom: 0;
    min-height: unset;
  }

  .child-card-icon {
    font-size: 1.1rem;
    line-height: 1;
    flex-shrink: 0;
  }

  .child-card-name {
    font-size: .9rem;
    font-weight: 600;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }


  /* ── Área de associações (rodapé) ─────────────────────── */
  .assoc-area {
    margin-top: 2rem;
  }

  .assoc-divider {
    display: flex;
    align-items: center;
    gap: .75rem;
    margin-bottom: 1rem;
  }

  .assoc-divider::before,
  .assoc-divider::after {
    content: '';
    flex: 1;
    height: 1px;
    background: var(--border);
  }

  .assoc-divider-label {
    font-size: .7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .08em;
    color: var(--text-muted);
    white-space: nowrap;
  }

  .assoc-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 1.25rem 1.5rem;
  }

  @media (max-width: 900px) {
    .assoc-grid { grid-template-columns: repeat(2, 1fr); }
  }

  @media (max-width: 700px) {
    .assoc-grid { grid-template-columns: 1fr; }
  }

  .assoc-col {
    display: flex;
    flex-direction: column;
    gap: .25rem;
    min-width: 0;
  }

  .assoc-col-title {
    font-size: .75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: .06em;
    color: var(--text-muted);
    margin-bottom: .4rem;
  }

  .assoc-item {
    display: flex;
    align-items: center;
    gap: .4rem;
    padding: .2rem 0;
    font-size: .875rem;
    min-width: 0;
  }

  .assoc-item-label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .assoc-item-label.link-title,
  .assoc-item-label.url-link,
  .assoc-item-label.att-link {
    color: var(--accent);
    cursor: pointer;
  }

  .assoc-item .row-btn {
    flex-shrink: 0;
    opacity: 0;
    font-size: .85rem;
    color: var(--text-muted);
    padding: .1rem .25rem;
    border-radius: 3px;
    transition: opacity .12s;
  }
  .assoc-item:hover .row-btn { opacity: 1; }
  .assoc-item .row-btn:hover { background: var(--bg-hover); color: var(--text); }
  .assoc-item .row-btn-del:hover { color: var(--danger); }
  .assoc-item .row-btn-edit:hover { color: var(--accent); }

  .url-edit-mode {
    flex-direction: column;
    align-items: stretch;
    gap: .3rem;
  }

  .url-edit-actions {
    display: flex;
    gap: .25rem;
    justify-content: flex-end;
  }

  .url-edit-btn {
    padding: .2rem .55rem;
    font-size: .8rem;
    border-radius: 4px;
    cursor: pointer;
  }
  .url-edit-btn-save {
    background: var(--accent);
    color: #fff;
    border: 1px solid var(--accent);
  }
  .url-edit-btn-save:disabled { opacity: .45; cursor: not-allowed; }
  .url-edit-btn-cancel {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-muted);
  }
  .url-edit-btn-cancel:hover { color: var(--text); border-color: var(--text-muted); }

  .assoc-item-label.link-title:hover,
  .assoc-item-label.url-link:hover,
  .assoc-item-label.att-link:hover { text-decoration: underline; }

  .assoc-empty {
    font-size: .8rem;
    color: var(--text-muted);
    font-style: italic;
    padding: .2rem 0 .4rem;
  }

  .assoc-date-created {
    font-size: .75rem;
    color: var(--text-muted);
    margin: 0 0 .5rem;
  }

  .assoc-date-label {
    font-weight: 600;
  }

  .assoc-date-row {
    display: flex;
    flex-direction: column;
    gap: .35rem;
    margin-bottom: .5rem;
  }

  .assoc-date-select {
    font-size: .8rem;
    padding: .25rem .4rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text);
    cursor: pointer;
    width: 100%;
  }

  .assoc-date-select:disabled {
    opacity: .4;
    cursor: not-allowed;
  }

  .assoc-date-display {
    font-size: .85rem;
    font-weight: 600;
    color: var(--text);
    margin: .2rem 0 .5rem;
  }

  .assoc-clear-date-btn {
    display: inline-flex;
    align-items: center;
    margin-top: .25rem;
    padding: .25rem .55rem;
    font-size: .75rem;
    border: 1px dashed var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
    background: transparent;
  }

  .assoc-clear-date-btn:hover {
    border-color: var(--text-muted);
    color: var(--text);
  }

  .assoc-add-btn {
    display: inline-flex;
    align-items: center;
    gap: .3rem;
    margin-top: .5rem;
    padding: .3rem .6rem;
    font-size: .8rem;
    border: 1px dashed var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
    background: transparent;
    transition: color .12s, border-color .12s;
  }

  .assoc-add-btn:hover { color: var(--accent); border-color: var(--accent); }
  .assoc-add-btn:disabled { opacity: .45; cursor: not-allowed; }

  /* Search wrap inside assoc column */
  .link-search-wrap {
    position: relative;
    display: block;
    margin-bottom: .25rem;
  }

  .assoc-search-input {
    width: 100%;
    padding: .3rem .5rem;
    font-size: .85rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: transparent;
    box-sizing: border-box;
  }

  .assoc-search-input:focus { outline: none; border-color: var(--accent); }

  .link-dropdown {
    position: fixed;
    min-width: 320px;
    max-width: 560px;
    background: var(--bg-panel);
    border: 1px solid var(--border);
    border-radius: 5px;
    z-index: 1000;
    box-shadow: 0 4px 16px rgba(0,0,0,.28);
    max-height: 260px;
    overflow-y: auto;
  }

  .link-option {
    padding: .4rem .75rem;
    font-size: .875rem;
    cursor: pointer;
    white-space: normal;
    word-break: break-word;
    line-height: 1.35;
  }

  .link-option:hover { background: var(--bg-hover); }

  .broken-badge {
    font-size: .7rem;
    background: var(--bg-hover);
    padding: .1rem .3rem;
    border-radius: 3px;
    margin-left: .25rem;
  }

  .url-add-row {
    display: flex;
    flex-direction: column;
    gap: .35rem;
    margin-top: .5rem;
  }

  .url-input,
  .url-title-input {
    width: 100%;
    padding: .3rem .5rem;
    font-size: .85rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: transparent;
    box-sizing: border-box;
  }

  .url-input:focus,
  .url-title-input:focus { outline: none; border-color: var(--accent); }

  .hidden-input { display: none; }

  /* ── Attachment preview modal ───────────────────────── */
  .preview-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,.72);
    z-index: 2000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
  }

  .preview-box {
    display: flex;
    flex-direction: column;
    background: var(--bg-panel, var(--bg-sidebar));
    border-radius: 10px;
    overflow: hidden;
    max-width: min(92vw, 1100px);
    max-height: 90vh;
    width: 100%;
    box-shadow: 0 24px 64px rgba(0,0,0,.5);
  }

  .preview-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: .75rem;
    padding: .6rem 1rem;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .preview-title {
    font-size: .875rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }

  .preview-actions {
    display: flex;
    align-items: center;
    gap: .5rem;
    flex-shrink: 0;
  }

  .preview-dl-btn {
    font-size: .8rem;
    color: var(--accent);
    text-decoration: none;
    padding: .2rem .5rem;
    border: 1px solid var(--accent);
    border-radius: 4px;
  }
  .preview-dl-btn:hover { background: var(--accent); color: #fff; }

  .preview-close {
    font-size: 1rem;
    color: var(--text-muted);
    background: none;
    border: none;
    cursor: pointer;
    padding: .2rem .4rem;
    border-radius: 4px;
    line-height: 1;
  }
  .preview-close:hover { background: var(--bg-hover); color: var(--text); }

  .preview-body {
    flex: 1;
    overflow: auto;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 0;
    background: var(--bg-main, #111);
  }

  .preview-img {
    max-width: 100%;
    max-height: calc(90vh - 56px);
    object-fit: contain;
    display: block;
  }

  .preview-pdf {
    width: 100%;
    height: calc(90vh - 56px);
    border: none;
    display: block;
  }

  .preview-video {
    max-width: 100%;
    max-height: calc(90vh - 56px);
    display: block;
  }

  .preview-audio-wrap,
  .preview-download-wrap {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
    padding: 2.5rem 2rem;
    background: var(--bg-panel, var(--bg-sidebar));
  }

  .preview-big-icon {
    font-size: 4rem;
    line-height: 1;
  }

  .preview-file-name {
    font-size: .95rem;
    font-weight: 600;
    text-align: center;
    word-break: break-all;
  }

  .preview-file-size {
    font-size: .8rem;
    color: var(--text-muted);
  }

  .preview-audio {
    width: 320px;
    max-width: 100%;
  }

  .preview-dl-large {
    margin-top: .5rem;
    text-decoration: none;
  }

  .preview-markdown {
    padding: 1.5rem 2rem;
    max-height: calc(90vh - 56px);
    overflow-y: auto;
    line-height: 1.7;
    width: 100%;
    background: var(--bg);
    color: var(--text);
    align-self: stretch;
  }
  .preview-markdown :global(h1), .preview-markdown :global(h2), .preview-markdown :global(h3),
  .preview-markdown :global(h4), .preview-markdown :global(h5), .preview-markdown :global(h6) {
    font-weight: 600; margin: 1.2em 0 .4em;
  }
  .preview-markdown :global(h1) { font-size: 1.5rem; }
  .preview-markdown :global(h2) { font-size: 1.25rem; }
  .preview-markdown :global(h3) { font-size: 1.1rem; }
  .preview-markdown :global(p) { margin: .6em 0; }
  .preview-markdown :global(pre) { background: var(--bg-hover); padding: .75rem 1rem; border-radius: var(--radius); overflow-x: auto; font-size: .85rem; }
  .preview-markdown :global(code) { font-family: monospace; font-size: .875em; background: var(--bg-hover); padding: .1em .3em; border-radius: 3px; }
  .preview-markdown :global(pre code) { background: none; padding: 0; }
  .preview-markdown :global(ul), .preview-markdown :global(ol) { padding-left: 1.5rem; margin: .5em 0; }
  .preview-markdown :global(li) { margin: .25em 0; }
  .preview-markdown :global(blockquote) { border-left: 3px solid var(--accent); padding-left: .75rem; color: var(--text-muted); margin: .75em 0; }
  .preview-markdown :global(a) { color: var(--accent); }
  .preview-markdown :global(table) { border-collapse: collapse; width: 100%; }
  .preview-markdown :global(th), .preview-markdown :global(td) { border: 1px solid var(--border); padding: .4rem .6rem; }
  .preview-markdown :global(th) { background: var(--bg-hover); font-weight: 600; }
  .preview-markdown :global(hr) { border: none; border-top: 1px solid var(--border); margin: 1em 0; }

  /* ── Attachment thumbnail grid ──────────────────────── */
  .att-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(88px, 1fr));
    gap: .5rem;
    margin-bottom: .4rem;
  }

  .att-card {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    background: var(--bg-secondary, var(--bg-sidebar));
  }

  .att-preview {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 72px;
    background: var(--bg-hover);
    text-decoration: none;
    overflow: hidden;
  }

  .att-thumb {
    width: 100%;
    height: 72px;
    object-fit: cover;
    display: block;
    transition: opacity .15s;
  }

  .att-thumb:hover { opacity: .85; }

  .att-icon {
    font-size: 2rem;
    line-height: 1;
    transition: transform .15s;
  }

  .att-preview:hover .att-icon { transform: scale(1.15); }

  .att-card-footer {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 3px 5px;
    border-top: 1px solid var(--border);
  }

  .att-name {
    flex: 1;
    font-size: .68rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-muted);
  }

  .att-del-btn {
    flex-shrink: 0;
    font-size: .75rem;
    color: var(--text-muted);
    background: none;
    border: none;
    cursor: pointer;
    padding: 0 2px;
    line-height: 1;
  }

  .att-del-btn:hover { color: var(--text); }

  /* ── Desktop: layout row com painel direito ───────────── */
  .content-pane {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    padding: 1.5rem 2rem;
  }

  .assoc-pane {
    display: flex;
    flex-direction: column;
    flex: 1;
    overflow-y: auto;
  }

  .assoc-pane .assoc-grid { grid-template-columns: 1fr; }
  .assoc-pane .assoc-area { margin-top: 0; padding: 1rem; }
  .assoc-pane .children-area { padding: 1rem 1rem 0; border-bottom: 1px solid var(--border); margin-bottom: 0; }

  .mobile-tab-bar { display: none; }
  .mobile-fab-edit { display: none; }
  .mobile-assoc-header { display: none; }
  .tb-done-mobile { color: var(--accent); font-weight: 700; }

  @media (max-width: 640px) {
    .content-pane {
      display: flex;
      flex-direction: column;
      flex: 1;
      overflow-y: auto;
      min-height: 0;
      padding: .75rem 1rem 1rem;
    }

    .assoc-pane {
      display: flex;
      flex-direction: column;
      flex: 1;
      overflow-y: auto;
      min-height: 0;
      padding: .75rem 1rem 64px;
    }

    .assoc-pane .assoc-area { padding: 0; }
    .assoc-pane .children-area { padding: 0; border-bottom: none; }

    .mobile-pane-hidden { display: none !important; }
    .mobile-toolbar-hidden { display: none !important; }

    /* Título em linha própria: os 10 botões da title-row são flex-shrink:0 e
       comprimiam o input a 0px em telas de 375px, tornando-o intocável. */
    .title-row { flex-wrap: wrap; }
    .doc-title { flex: 1 1 100%; order: -1; }

    .mobile-tab-bar {
      display: flex;
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      height: 52px;
      background: var(--bg-panel);
      border-top: 1px solid var(--border);
      z-index: 30;
      box-shadow: 0 -2px 8px rgba(0,0,0,.08);
    }

    .mobile-tab-bar button {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 2px;
      background: none;
      border: none;
      cursor: pointer;
      color: var(--text-muted);
      transition: color .15s;
      height: 52px;
    }

    .mobile-tab-bar button.active { color: var(--accent); font-weight: 600; }
    .tab-icon { font-size: 1.2rem; line-height: 1; }
    .tab-label { font-size: .68rem; line-height: 1; }

    .mobile-fab-edit {
      display: flex;
      position: fixed;
      bottom: 64px;
      right: 1rem;
      width: 52px;
      height: 52px;
      border-radius: 50%;
      background: var(--accent);
      color: #fff;
      font-size: 1.4rem;
      border: none;
      cursor: pointer;
      box-shadow: 0 3px 14px rgba(0,0,0,.3);
      align-items: center;
      justify-content: center;
      z-index: 29;
      transition: transform .1s;
    }
    .mobile-fab-edit:active { transform: scale(.93); }

    .mobile-assoc-header {
      display: flex;
      align-items: center;
      gap: .5rem;
      padding: .25rem .25rem .75rem;
      border-bottom: 1px solid var(--border);
      margin-bottom: .75rem;
    }
    .mobile-assoc-header i { font-size: 1.25rem; flex-shrink: 0; }
    .mobile-assoc-title {
      font-size: .9rem;
      font-weight: 600;
      color: var(--text);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }
</style>
