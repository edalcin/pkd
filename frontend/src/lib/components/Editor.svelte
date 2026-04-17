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

    <!-- ── Área de associações ───────────────────────────────────── -->
    <div class="assoc-area">
      <div class="assoc-divider">
        <span class="assoc-divider-label">Associações</span>
      </div>

      <div class="assoc-grid">

        <!-- Coluna 1: Notas relacionadas -->
        <section class="assoc-col">
          <h4 class="assoc-col-title">📄 Notas relacionadas</h4>

          <!-- busca para adicionar -->
          <div class="link-search-wrap">
            <input
              class="assoc-search-input"
              type="text"
              bind:value={linkQuery}
              bind:this={linkInputEl}
              oninput={onLinkInput}
              onblur={() => setTimeout(closeLinkSearch, 150)}
              placeholder="Buscar nota para relacionar…"
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
            <p class="assoc-empty">Nenhuma nota relacionada</p>
          {/if}
        </section>

        <!-- Coluna 2: Arquivos -->
        <section class="assoc-col">
          <h4 class="assoc-col-title">📎 Arquivos</h4>

          {#each attachments as att}
            <div class="assoc-item">
              <a href="/api/attachments/{att.id}" target="_blank" class="assoc-item-label att-link">
                {att.original_name || `Anexo #${att.id}`}
              </a>
              <button class="row-btn row-btn-del" onclick={() => deleteAttachment(att.id)} title="Remover">×</button>
            </div>
          {/each}

          {#if attachments.length === 0}
            <p class="assoc-empty">Nenhum arquivo anexado</p>
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
    background: var(--bg-sidebar);
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
</style>
