<script>
  import { onMount, onDestroy } from 'svelte'
  import { Editor } from '@tiptap/core'
  import StarterKit from '@tiptap/starter-kit'
  import Image from '@tiptap/extension-image'
  import { DocLink } from '../editor/doclink-extension.js'
  import { saveDoc, loadDoc } from '../stores/documents.js'
  import { setDocumentTags } from '../stores/tags.js'
  import { apiFetch, apiGet, apiDelete } from '../api.js'

  let { docId } = $props()

  let doc = $state(null)
  let titleValue = $state('')
  let tagInput = $state('')
  let docTags = $state([])
  let backlinks = $state([])
  let attachments = $state([])
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

  onMount(() => loadDocument())

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
      await loadBacklinks()
      attachments = doc?.attachment_ids?.map(id => ({ id })) || []
    } finally {
      loading = false
    }
  }

  async function loadBacklinks() {
    try {
      const resp = await apiGet(`/api/documents/${docId}/links`)
      backlinks = resp.incoming || []
    } catch { backlinks = [] }
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
  <div class="editor-area" onkeydown={handleKeydown} role="region" aria-label="Editor de documento">
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

    <!-- Formatting toolbar -->
    {#if editorReady}
      <!-- editorTick drives re-evaluation of isActive() on every transaction -->
      {#key editorTick}
      <div class="toolbar" role="toolbar" aria-label="Formatação">
        <!-- Headings -->
        <button class="tb-btn {isActive('heading', {level:1}) ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleHeading({level:1}))} title="Título 1" aria-pressed={isActive('heading',{level:1})}>H1</button>
        <button class="tb-btn {isActive('heading', {level:2}) ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleHeading({level:2}))} title="Título 2" aria-pressed={isActive('heading',{level:2})}>H2</button>
        <button class="tb-btn {isActive('heading', {level:3}) ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleHeading({level:3}))} title="Título 3" aria-pressed={isActive('heading',{level:3})}>H3</button>

        <div class="tb-sep" role="separator"></div>

        <!-- Inline styles -->
        <button class="tb-btn {isActive('bold') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleBold())} title="Negrito (Ctrl+B)" aria-pressed={isActive('bold')}><strong>B</strong></button>
        <button class="tb-btn {isActive('italic') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleItalic())} title="Itálico (Ctrl+I)" aria-pressed={isActive('italic')}><em>I</em></button>
        <button class="tb-btn {isActive('strike') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleStrike())} title="Tachado" aria-pressed={isActive('strike')}><s>S</s></button>
        <button class="tb-btn {isActive('code') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleCode())} title="Código inline" aria-pressed={isActive('code')}><code>`</code></button>

        <div class="tb-sep" role="separator"></div>

        <!-- Blocks -->
        <button class="tb-btn {isActive('bulletList') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleBulletList())} title="Lista" aria-pressed={isActive('bulletList')}>☰</button>
        <button class="tb-btn {isActive('orderedList') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleOrderedList())} title="Lista numerada" aria-pressed={isActive('orderedList')}>1.</button>
        <button class="tb-btn {isActive('blockquote') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleBlockquote())} title="Citação" aria-pressed={isActive('blockquote')}>"</button>
        <button class="tb-btn {isActive('codeBlock') ? 'active' : ''}"
          onclick={() => fmt(c => c.toggleCodeBlock())} title="Bloco de código" aria-pressed={isActive('codeBlock')}>&lt;/&gt;</button>

        <div class="tb-sep" role="separator"></div>

        <!-- Utility -->
        <button class="tb-btn" onclick={() => fmt(c => c.setHorizontalRule())} title="Linha horizontal">—</button>
        <button class="tb-btn" onclick={() => fmt(c => c.undo())} title="Desfazer (Ctrl+Z)">↩</button>
        <button class="tb-btn" onclick={() => fmt(c => c.redo())} title="Refazer (Ctrl+Y)">↪</button>

        <div class="tb-spacer"></div>

        <!-- Save -->
        <button class="tb-btn tb-save {saving ? 'saving' : ''}"
          onclick={performSave} disabled={saving} title="Salvar (Ctrl+S)">
          {saving ? '⏳' : '💾'} Salvar
        </button>
      </div>
      {/key}
    {/if}

    <!-- TipTap editor -->
    <div class="tiptap-editor" use:mountEditor></div>

    <!-- Backlinks -->
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
</style>
