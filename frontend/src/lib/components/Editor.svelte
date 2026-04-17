<script>
  import { onDestroy } from 'svelte'
  import { Editor } from '@tiptap/core'
  import StarterKit from '@tiptap/starter-kit'
  import Image from '@tiptap/extension-image'
  import { DocLink } from '../editor/doclink-extension.js'
  import { saveDoc, loadDoc } from '../stores/documents.js'
  import { setDocumentTags } from '../stores/tags.js'
  import { apiFetch, apiGet, apiPost, apiDelete } from '../api.js'

  let { docId } = $props()

  let doc = $state(null)
  let titleValue = $state('')
  let tagInput = $state('')
  let docTags = $state([])
  let backlinks = $state([])
  let outgoingLinks = $state([])
  let attachments = $state([])
  let docUrls = $state([])
  let urlInput = $state('')
  let urlTitleInput = $state('')
  let urlAdding = $state(false)

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

  // Tags
  async function addTag(e) {
    if (e.key !== 'Enter' && e.key !== ',') return
    e.preventDefault()
    const name = tagInput.trim().toLowerCase().replace(/^#/, '').replace(/\s+/g, '-')
    if (name && !docTags.includes(name)) {
      docTags = [...docTags, name]
      await setDocumentTags(doc.id, docTags)
    }
    tagInput = ''
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
  }

  // Svelte action: mounts the TipTap editor on the DOM node
  function mountEditor(node) {
    editorInstance = new Editor({
      element: node,
      extensions: [
        StarterKit,
        Image.configure({ inline: true, allowBase64: true }),
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
          <span class="tag-chip active">
            #{tag}
            <button class="tag-remove" onclick={() => removeTag(tag)} aria-label="Remover tag">×</button>
          </span>
        {/each}
        <input
          class="tag-input"
          type="text"
          bind:value={tagInput}
          onkeydown={addTag}
          placeholder="+ tag"
          aria-label="Adicionar tag"
        />
        <!-- Relate note search (inline with tags) -->
        <div class="link-search-wrap">
          <input
            class="tag-input link-search-input"
            type="text"
            bind:value={linkQuery}
            bind:this={linkInputEl}
            oninput={onLinkInput}
            onblur={() => setTimeout(closeLinkSearch, 150)}
            placeholder="+ relacionar"
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
                >
                  📄 {hit.title}
                </div>
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

    <!-- Related notes (outgoing links) -->
    {#if outgoingLinks.length > 0}
    <div class="links-panel">
      <h3>Notas relacionadas ({outgoingLinks.length})</h3>
      {#each outgoingLinks as link}
        <div class="link-item">
          <span
            class="link-title"
            role="button"
            tabindex="0"
            onclick={() => window.location.hash = `/doc/${link.target_id}`}
            onkeydown={e => e.key === 'Enter' && (window.location.hash = `/doc/${link.target_id}`)}
          >→ {link.target_title}</span>
          <button class="row-btn row-btn-del" onmousedown={e => { e.preventDefault(); removeLink(link.id) }} title="Remover relação">×</button>
        </div>
      {/each}
    </div>
    {/if}

    <!-- Backlinks (incoming) -->
    {#if backlinks.length > 0}
      <div class="backlinks-panel">
        <h3>Referenciado por ({backlinks.length})</h3>
        {#each backlinks as link}
          <div
            class="backlink-item {link.target_trashed ? 'broken' : ''}"
            onclick={() => !link.target_trashed && (window.location.hash = `/doc/${link.source_id}`)}
            onkeydown={e => e.key === 'Enter' && !link.target_trashed && (window.location.hash = `/doc/${link.source_id}`)}
            role="button"
            tabindex={link.target_trashed ? -1 : 0}
          >
            📄 {link.source_title}
            {#if link.target_trashed}<span class="broken-badge">excluído</span>{/if}
          </div>
        {/each}
      </div>
    {/if}

    <!-- Attachments -->
    {#if attachments.length > 0}
      <div class="attachments-panel">
        <h3>Anexos</h3>
        {#each attachments as att}
          <div class="attachment-item">
            <a href="/api/attachments/{att.id}" target="_blank" class="att-link">
              📎 {att.original_name || `Anexo #${att.id}`}
            </a>
            <button class="row-btn row-btn-del" onclick={() => deleteAttachment(att.id)}>×</button>
          </div>
        {/each}
      </div>
    {/if}

    <!-- File upload -->
    <div class="upload-row">
      <label class="btn btn-ghost upload-label">
        📎 Anexar arquivo
        <input type="file" class="hidden-input" onchange={handleFileUpload} />
      </label>
    </div>

    <!-- External URLs -->
    <div class="urls-panel">
      <h3>Links externos ({docUrls.length})</h3>
      {#each docUrls as u}
        <div class="url-item">
          <a href={u.url} target="_blank" rel="noopener noreferrer" class="url-link">
            🔗 {u.title || u.url}
          </a>
          {#if u.title}<span class="url-subtitle">{u.url}</span>{/if}
          <button class="row-btn row-btn-del" onclick={() => deleteURL(u.id)} title="Remover link">×</button>
        </div>
      {/each}
      <div class="url-add-row">
        <input
          class="url-input"
          type="url"
          bind:value={urlInput}
          placeholder="https://exemplo.com"
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
        <button class="btn btn-ghost" onclick={addURL} disabled={urlAdding || !urlInput.trim()}>
          + Link
        </button>
      </div>
    </div>
  </div>

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
    flex-wrap: wrap;
    gap: 2px;
    padding: .25rem .25rem;
    margin-bottom: .5rem;
    background: var(--bg-secondary, var(--bg-sidebar));
    border: 1px solid var(--border);
    border-radius: 6px;
  }

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

  .doc-header { margin-bottom: .75rem; }

  .tag-input {
    border: none;
    border-bottom: 1px solid var(--border);
    border-radius: 0;
    padding: .1rem .25rem;
    font-size: .8rem;
    width: 80px;
    background: transparent;
  }

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

  /* ── Related notes / links ──────────────────────────── */
  .links-panel {
    margin-top: 1rem;
    padding-top: .75rem;
    border-top: 1px solid var(--border);
  }

  /* Wrapper sits inline in the doc-meta row */
  .link-search-wrap {
    position: relative;
    display: inline-block;
  }

  .link-dropdown {
    position: fixed;
    min-width: 380px;
    max-width: 600px;
    background: var(--bg-sidebar);
    border: 1px solid var(--border);
    border-radius: 5px;
    z-index: 1000;
    box-shadow: 0 4px 12px rgba(0,0,0,.25);
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

  .link-option:hover {
    background: var(--bg-hover);
  }

  .link-item {
    display: flex;
    align-items: center;
    gap: .4rem;
    padding: .2rem 0;
    font-size: .9rem;
  }

  .link-title {
    flex: 1;
    cursor: pointer;
    color: var(--accent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .link-title:hover { text-decoration: underline; }

  .attachments-panel {
    margin-top: 1rem;
    padding-top: .75rem;
    border-top: 1px solid var(--border);
  }

  .attachments-panel h3,
  .backlinks-panel h3 {
    font-size: .8rem;
    color: var(--text-muted);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .05em;
    margin-bottom: .5rem;
  }

  .attachment-item {
    display: flex;
    align-items: center;
    gap: .5rem;
    padding: .25rem 0;
    font-size: .9rem;
  }

  .att-link { color: var(--accent); }
  .upload-row { margin-top: .75rem; }
  .upload-label { cursor: pointer; }
  .hidden-input { display: none; }

  .urls-panel {
    margin-top: 1rem;
    padding-top: .75rem;
    border-top: 1px solid var(--border);
  }

  .urls-panel h3 {
    font-size: .8rem;
    color: var(--text-muted);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .05em;
    margin-bottom: .5rem;
  }

  .url-item {
    display: flex;
    align-items: center;
    gap: .4rem;
    padding: .2rem 0;
    font-size: .9rem;
  }

  .url-link {
    color: var(--accent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }

  .url-link:hover { text-decoration: underline; }

  .url-subtitle {
    font-size: .75rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 150px;
    flex-shrink: 0;
  }

  .url-add-row {
    display: flex;
    gap: .4rem;
    margin-top: .5rem;
    flex-wrap: wrap;
  }

  .url-input {
    flex: 2;
    min-width: 160px;
    padding: .3rem .5rem;
    font-size: .85rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-main, transparent);
  }

  .url-title-input {
    flex: 1;
    min-width: 100px;
    padding: .3rem .5rem;
    font-size: .85rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-main, transparent);
  }
</style>
