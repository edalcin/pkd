<script>
  import { onMount, onDestroy } from 'svelte'
  import { createEditor, EditorContent } from 'svelte-tiptap'
  import StarterKit from '@tiptap/starter-kit'
  import Image from '@tiptap/extension-image'
  import Suggestion from '@tiptap/suggestion'
  import { DocLink } from '../editor/doclink-extension.js'
  import { buildLinkSuggestion } from '../editor/link-suggestion.js'
  import { saveDoc, loadDoc } from '../stores/documents.js'
  import { tags, setDocumentTags, loadTags } from '../stores/tags.js'
  import { apiFetch, apiGet, apiPost, apiDelete } from '../api.js'

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

  let autoSaveTimer = null
  let editor = $state(null)

  // Build link suggestion extension
  const linkSuggestion = buildLinkSuggestion()

  // Build DocLinkWithSuggestion
  const DocLinkWithSuggestion = DocLink.extend({
    addInputRules() { return [] },
  })

  const LinkExtension = Suggestion.configure ? DocLink : DocLink

  onMount(async () => {
    await loadDocument()
  })

  onDestroy(() => {
    clearTimeout(autoSaveTimer)
    if (editor) get(editor)?.destroy()
  })

  async function loadDocument() {
    loading = true
    try {
      doc = await loadDoc(Number(docId))
      titleValue = doc.title
      docTags = doc.tags || []
      await loadBacklinks()
      await loadAttachments()
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

  async function loadAttachments() {
    // Attachment IDs are on doc.attachment_ids; fetch metadata from store
    attachments = doc?.attachment_ids?.map(id => ({ id })) || []
  }

  // Reload when docId prop changes
  $effect(() => {
    if (docId) loadDocument()
  })

  // Auto-save: triggered 2 seconds after the user stops typing
  function scheduleAutoSave() {
    clearTimeout(autoSaveTimer)
    autoSaveTimer = setTimeout(performSave, 2000)
  }

  async function performSave() {
    if (!doc || !editor) return
    const editorInstance = typeof editor === 'object' && 'subscribe' in editor
      ? (() => { let v; editor.subscribe(e => v = e)(); return v })()
      : editor
    if (!editorInstance) return

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
    } catch (err) {
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

  // Attachments
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

  // Image paste handler
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

  function editorReady(node) {
    // Create editor after the container is in the DOM
    const e = createEditor({
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
    })
    editor = e
    return {
      destroy: () => {
        clearTimeout(autoSaveTimer)
        let v; e.subscribe(inst => v = inst)()
        v?.destroy()
      },
    }
  }
</script>

{#if loading}
  <div class="editor-area">
    <div class="empty-state"><div class="spinner"></div></div>
  </div>
{:else if doc}
  <div class="editor-area">
    <!-- Title + meta row -->
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
        <!-- Tags -->
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

        <!-- Save status -->
        <span class="save-status">
          {#if saving}⏳{:else if saveError}❌ {saveError}{:else}✓{/if}
        </span>
      </div>
    </div>

    <!-- TipTap editor -->
    <div class="tiptap-editor" use:editorReady></div>

    <!-- Backlinks panel -->
    {#if backlinks.length > 0}
      <div class="backlinks-panel">
        <h3>Referenciado por ({backlinks.length})</h3>
        {#each backlinks as link}
          <div
            class="backlink-item {link.target_trashed ? 'broken' : ''}"
            onclick={() => !link.target_trashed && (window.location.hash = `/doc/${link.source_id}`)}
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
      <label class="btn btn-ghost upload-label" title="Anexar arquivo">
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
          <button class="btn btn-ghost" onclick={handleConflictReload}>
            Descartar e recarregar
          </button>
          <button class="btn btn-primary" onclick={handleConflictOverwrite}>
            Sobrescrever com minha versão
          </button>
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

  .hidden-input {
    display: none;
  }
</style>
