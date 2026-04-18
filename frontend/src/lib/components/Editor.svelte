<script>
  import { onDestroy } from 'svelte'
  import { Editor } from '@tiptap/core'
  import StarterKit from '@tiptap/starter-kit'
  import Image from '@tiptap/extension-image'
  import Table from '@tiptap/extension-table'
  import TableRow from '@tiptap/extension-table-row'
  import TableCell from '@tiptap/extension-table-cell'
  import TableHeader from '@tiptap/extension-table-header'
  import Highlight from '@tiptap/extension-highlight'
  import TextAlign from '@tiptap/extension-text-align'
  import { DocLink } from '../editor/doclink-extension.js'
  import { saveDoc, loadDoc, linksRefreshSignal } from '../stores/documents.js'
  import { setDocumentTags, loadTags, tags as allTags } from '../stores/tags.js'
  import { apiFetch, apiGet, apiPost, apiDelete } from '../api.js'

  let { docId } = $props()

  let doc = $state(null)
  let titleValue = $state('')
  let tagInput = $state('')
  let tagSuggestions = $state([])
  let tagSuggestionsOpen = $state(false)
  let tagInputEl = $state(null)
  let tagDropdownStyle = $state('')
  let docTags = $state([])
  let backlinks = $state([])
  let outgoingLinks = $state([])
  let attachments = $state([])
  let docUrls = $state([])
  let children = $state([])
  let urlInput = $state('')
  let urlTitleInput = $state('')
  let urlAdding = $state(false)

  // Map tag name → color, derived from the global tags store
  const tagColorMap = $derived(
    Object.fromEntries(($allTags || []).map(t => [t.name, t.color || '']))
  )

  let highlightColor = $state('#fef08a') // default yellow

  function insertImageByURL() {
    const url = prompt('URL da imagem:')
    if (url?.trim()) fmt(c => c.setImage({ src: url.trim() }))
  }

  // Attachment preview modal
  let previewAtt = $state(null)

  function openPreview(att) {
    previewAtt = att
  }
  function closePreview() {
    previewAtt = null
  }
  function previewType(mime) {
    if (!mime) return 'download'
    if (mime.startsWith('image/')) return 'image'
    if (mime === 'application/pdf') return 'pdf'
    if (mime.startsWith('audio/')) return 'audio'
    if (mime.startsWith('video/')) return 'video'
    return 'download'
  }

  // Link search state
  let linkQuery = $state('')
  let linkResults = $state([])
  let linkSearchOpen = $state(false)
  let linkSearchTimer = null
  let dropdownStyle = $state('')
  let linkInputEl = $state(null)
  let saving = $state(false)
  let saveError = $state('')
  let conflictData = $state(null)
  let loading = $state(true)

  // TipTap Editor instance (not reactive — managed manually)
  let editorInstance = null
  let autoSaveTimer = null

  // editorReady signals that the editor has been mounted (triggers toolbar render).
  // editorTick increments on each transaction to keep toolbar active-states fresh.
  let editorReady = $state(false)
  let editorTick = $state(0)

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

  async function loadDocument() {
    loading = true
    try {
      doc = await loadDoc(Number(docId))
      titleValue = doc.title
      docTags = doc.tags || []
      loadTags()   // populate allTags store for autocomplete (fire-and-forget)
      await loadLinks()
      try {
        const atts = await apiGet(`/api/documents/${docId}/attachments`)
        attachments = atts || []
      } catch {
        attachments = doc?.attachment_ids?.map(id => ({ id })) || []
      }
      try {
        docUrls = await apiGet(`/api/documents/${docId}/urls`)
      } catch {
        docUrls = []
      }
      try {
        children = await apiGet(`/api/documents/${docId}/children`) || []
      } catch {
        children = []
      }
    } finally {
      loading = false
    }
  }

  async function loadLinks() {
    try {
      const resp = await apiGet(`/api/documents/${docId}/links`)
      backlinks = resp.incoming || []
      outgoingLinks = resp.outgoing || []
    } catch {
      backlinks = []
      outgoingLinks = []
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
    if ($linksRefreshSignal && doc) loadLinks()
  })

  // Auto-save: 2 seconds after user stops typing
  function scheduleAutoSave() {
    clearTimeout(autoSaveTimer)
    autoSaveTimer = setTimeout(performSave, 2000)
  }

  async function performSave() {
    if (!doc || !editorInstance) return
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
        conflictData = result
        return
      }
      doc = result
    } catch {
      saveError = 'Erro ao salvar'
    } finally {
      saving = false
    }
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

  // ── Related notes (outgoing links) ────────────────────────────────────────

  function onLinkInput() {
    clearTimeout(linkSearchTimer)
    if (linkQuery.trim().length < 1) { linkResults = []; linkSearchOpen = false; return }
    linkSearchTimer = setTimeout(async () => {
      try {
        const hits = await apiGet(`/api/search?q=${encodeURIComponent(linkQuery)}`)
        const linked = new Set(outgoingLinks.map(l => l.target_id))
        linkResults = (hits || []).filter(h => h.id !== doc.id && !linked.has(h.id)).slice(0, 8)
        if (linkResults.length > 0) {
          // Position dropdown below the input using fixed coords
          if (linkInputEl) {
            const r = linkInputEl.getBoundingClientRect()
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
    outgoingLinks = outgoingLinks.filter(l => l.id !== linkId)
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

  // Image paste from clipboard
  function handleImagePaste(view, event) {
    const items = [...(event.clipboardData?.items || [])]
    const imageItem = items.find(i => i.type.startsWith('image/'))
    if (!imageItem) return false
    const file = imageItem.getAsFile()
    if (!file) return false
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

  // Ctrl+S saves the document from anywhere in the editor area
  function handleKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault()
      performSave()
    }
    if (e.key === 'Escape' && previewAtt) {
      closePreview()
    }
  }

  // Svelte action: mounts the TipTap editor on the DOM node
  function mountEditor(node) {
    editorInstance = new Editor({
      element: node,
      extensions: [
        StarterKit,
        Image.configure({ inline: true, allowBase64: true }),
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
        handlePaste: handleImagePaste,
        attributes: {
          class: 'ProseMirror',
          'data-placeholder': 'Comece a escrever…',
        },
      },
      onUpdate: () => scheduleAutoSave(),
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
    <!-- Title -->
    <div class="doc-header">
      <input
        class="doc-title"
        type="text"
        bind:value={titleValue}
        oninput={scheduleAutoSave}
        placeholder="Título do documento"
        aria-label="Título"
      />
      <div class="doc-meta">
        {#each docTags as tag}
          {@const c = tagColorMap[tag] || ''}
          <span
            class="tag-chip active"
            style={c ? `background:${c}; border-color:${c}; color:#fff` : ''}
          >
            #{tag}
            <button class="tag-remove" onclick={() => removeTag(tag)} aria-label="Remover tag">×</button>
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

    <!-- Formatting toolbar — always visible when a document is open.
         {#key editorTick} forces re-evaluation of isActive() on every
         TipTap transaction (selection change, content edit). -->
    {#key editorTick}
    <div class="toolbar" role="toolbar" aria-label="Formatação">
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

        <!-- Image by URL -->
        <button class="tb-btn" onmousedown={e => { e.preventDefault(); insertImageByURL() }} title="Inserir imagem por URL">🖼</button>

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

        <!-- Save -->
        <button class="tb-btn tb-save {saving ? 'saving' : ''}"
          onclick={performSave} disabled={saving} title="Salvar (Ctrl+S)">
          {saving ? '⏳' : '💾'} Salvar
        </button>
      </div>
    {/key}

    <!-- TipTap editor -->
    <div class="tiptap-editor" use:mountEditor></div>

    <!-- ── Sub-documentos ────────────────────────────────────────── -->
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
                {#if child.icon}<span class="child-card-icon">{child.icon}</span>{/if}
                <span class="child-card-name">{child.title || 'Sem título'}</span>
              </div>
              {#if child.body_text}
                <p class="child-card-preview">{child.body_text.slice(0, 160)}{child.body_text.length > 160 ? '…' : ''}</p>
              {:else}
                <p class="child-card-preview child-card-empty">Sem conteúdo</p>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- ── Área de associações ───────────────────────────────────── -->
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
              bind:this={linkInputEl}
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

          <!-- outgoing -->
          {#each outgoingLinks as link}
            <div class="assoc-item">
              <span
                class="assoc-item-label link-title"
                role="button" tabindex="0"
                onclick={() => window.location.hash = `/doc/${link.target_id}`}
                onkeydown={e => e.key === 'Enter' && (window.location.hash = `/doc/${link.target_id}`)}
              >→ {link.target_title}</span>
              <button class="row-btn row-btn-del" onmousedown={e => { e.preventDefault(); removeLink(link.id) }} title="Remover">×</button>
            </div>
          {/each}

          <!-- backlinks -->
          {#if backlinks.length > 0}
            <p class="assoc-sub-title">Referenciado por</p>
            {#each backlinks as link}
              <div
                class="assoc-item backlink-item {link.target_trashed ? 'broken' : ''}"
                onclick={() => !link.target_trashed && (window.location.hash = `/doc/${link.source_id}`)}
                onkeydown={e => e.key === 'Enter' && !link.target_trashed && (window.location.hash = `/doc/${link.source_id}`)}
                role="button" tabindex={link.target_trashed ? -1 : 0}
              >
                <span class="assoc-item-label">← {link.source_title}</span>
                {#if link.target_trashed}<span class="broken-badge">excluído</span>{/if}
              </div>
            {/each}
          {/if}

          {#if outgoingLinks.length === 0 && backlinks.length === 0}
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
            <div class="assoc-item">
              <a href={u.url} target="_blank" rel="noopener noreferrer" class="assoc-item-label url-link" title={u.url}>
                {u.title || u.url}
              </a>
              <button class="row-btn row-btn-del" onclick={() => deleteURL(u.id)} title="Remover">×</button>
            </div>
          {/each}

          {#if docUrls.length === 0}
            <p class="assoc-empty">Nenhum link externo</p>
          {/if}

          <div class="url-add-row">
            <input
              class="url-input"
              type="url"
              bind:value={urlInput}
              placeholder="https://…"
              aria-label="URL"
              onkeydown={e => e.key === 'Enter' && addURL()}
            />
            <input
              class="url-title-input"
              type="text"
              bind:value={urlTitleInput}
              placeholder="Título (opcional)"
              aria-label="Título do link"
              onkeydown={e => e.key === 'Enter' && addURL()}
            />
            <button class="assoc-add-btn" onclick={addURL} disabled={urlAdding || !urlInput.trim()}>
              + Adicionar
            </button>
          </div>
        </section>

      </div>
    </div>
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
          {#if previewType(previewAtt.mime_type) === 'image'}
            <img src="/api/attachments/{previewAtt.id}" alt={previewAtt.original_name} class="preview-img" />

          {:else if previewType(previewAtt.mime_type) === 'pdf'}
            <embed
              src="/api/attachments/{previewAtt.id}"
              type="application/pdf"
              class="preview-pdf"
            />

          {:else if previewType(previewAtt.mime_type) === 'audio'}
            <div class="preview-audio-wrap">
              <span class="preview-big-icon">🎵</span>
              <p class="preview-file-name">{previewAtt.original_name}</p>
              <!-- svelte-ignore a11y_media_has_caption -->
              <audio controls src="/api/attachments/{previewAtt.id}" class="preview-audio"></audio>
            </div>

          {:else if previewType(previewAtt.mime_type) === 'video'}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video controls src="/api/attachments/{previewAtt.id}" class="preview-video"></video>

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
{:else}
  <div class="editor-area empty-state">
    <span class="emoji">📝</span>
    <p>Selecione um documento ou crie um novo</p>
  </div>
{/if}

<style>
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

  .tb-save {
    gap: 4px;
    font-weight: 600;
    padding: 0 10px;
  }

  .tb-save.saving {
    opacity: .7;
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
    padding: .75rem 1rem;
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
    margin-bottom: .35rem;
    min-height: 1.4em;
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

  .child-card-preview {
    font-size: .78rem;
    color: var(--text-muted);
    line-height: 1.45;
    margin: 0;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .child-card-empty {
    font-style: italic;
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
    grid-template-columns: repeat(3, 1fr);
    gap: 1.25rem 1.5rem;
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

  .assoc-sub-title {
    font-size: .7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .05em;
    color: var(--text-muted);
    margin: .5rem 0 .2rem;
    opacity: .7;
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

  .assoc-item-label.link-title:hover,
  .assoc-item-label.url-link:hover,
  .assoc-item-label.att-link:hover { text-decoration: underline; }

  .assoc-empty {
    font-size: .8rem;
    color: var(--text-muted);
    font-style: italic;
    padding: .2rem 0 .4rem;
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
</style>
