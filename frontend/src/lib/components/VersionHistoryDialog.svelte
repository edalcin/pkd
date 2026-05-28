<script>
  import { listVersions, getVersion, restoreVersion } from '../stores/documents.js'

  let { docId, currentVersion, onClose, onRestored } = $props()

  let versions = $state([])
  let selected = $state(null)
  let preview = $state(null)
  let loadingList = $state(true)
  let loadingPreview = $state(false)
  let restoring = $state(false)
  let confirmRestore = $state(false)

  $effect(() => {
    if (docId) {
      loadingList = true
      listVersions(docId).then(data => {
        versions = data ?? []
        loadingList = false
      })
    }
  })

  async function selectVersion(v) {
    if (selected?.id === v.id) return
    selected = v
    preview = null
    confirmRestore = false
    loadingPreview = true
    preview = await getVersion(docId, v.id)
    loadingPreview = false
  }

  async function doRestore() {
    if (!selected) return
    restoring = true
    try {
      const updated = await restoreVersion(docId, selected.id, currentVersion)
      onRestored(updated)
    } finally {
      restoring = false
      confirmRestore = false
    }
  }

  function formatDate(isoStr) {
    if (!isoStr) return ''
    const d = new Date(isoStr)
    return d.toLocaleString('pt-BR', {
      day: '2-digit', month: '2-digit', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  }

  // Disable restore if the selected version is already the most-recent snapshot
  const isAlreadyCurrent = $derived(selected && versions.length > 0 && selected.id === versions[0].id)
</script>

<div class="modal-backdrop" onclick={onClose} role="dialog" aria-modal="true" aria-label="Histórico de versões">
  <div class="hist-modal" onclick={e => e.stopPropagation()}>
    <div class="hist-header">
      <h2>Histórico de versões</h2>
      <button class="close-btn" onclick={onClose} aria-label="Fechar"><i class="bx bx-x"></i></button>
    </div>

    <div class="hist-body">
      <!-- Left: version list -->
      <div class="hist-list">
        {#if loadingList}
          <p class="hist-empty">Carregando…</p>
        {:else if versions.length === 0}
          <p class="hist-empty">Nenhuma versão salva ainda.</p>
        {:else}
          {#each versions as v, i}
            <button
              class="version-item {selected?.id === v.id ? 'active' : ''}"
              onclick={() => selectVersion(v)}
            >
              <span class="version-icon"><i class="bx {v.icon || 'bx-file'}"></i></span>
              <span class="version-info">
                <span class="version-title">{v.title || 'Sem título'}</span>
                <span class="version-date">{formatDate(v.created_at)}</span>
              </span>
              {#if i === 0}
                <span class="version-badge">Atual</span>
              {/if}
            </button>
          {/each}
        {/if}
      </div>

      <!-- Right: preview -->
      <div class="hist-preview">
        {#if !selected}
          <p class="hist-empty">Selecione uma versão para visualizar.</p>
        {:else if loadingPreview}
          <p class="hist-empty">Carregando…</p>
        {:else if preview}
          <div class="preview-header">
            <span class="preview-meta">{formatDate(preview.created_at)}</span>
          </div>
          <div class="preview-content tiptap-editor">
            {@html preview.body_html}
          </div>
        {/if}
      </div>
    </div>

    <div class="hist-footer">
      {#if confirmRestore}
        <span class="confirm-text">Restaurar para <strong>{formatDate(selected?.created_at)}</strong>?</span>
        <button class="btn btn-ghost" onclick={() => confirmRestore = false} disabled={restoring}>Cancelar</button>
        <button class="btn btn-primary" onclick={doRestore} disabled={restoring}>
          {restoring ? 'Restaurando…' : 'Confirmar'}
        </button>
      {:else}
        <button class="btn btn-ghost" onclick={onClose}>Fechar</button>
        <button
          class="btn btn-primary"
          disabled={!selected || isAlreadyCurrent || restoring}
          onclick={() => confirmRestore = true}
        >
          Restaurar esta versão
        </button>
      {/if}
    </div>
  </div>
</div>

<style>
  .hist-modal {
    background: var(--bg-surface, var(--bg));
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 88vw;
    max-width: 1000px;
    height: 80vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0,0,0,.18);
  }

  .hist-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: .75rem 1.25rem;
    border-bottom: 1px solid var(--border);
  }
  .hist-header h2 { margin: 0; font-size: 1rem; }

  .close-btn {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 1.4rem;
    color: var(--text-muted);
    line-height: 1;
    padding: 0 .25rem;
  }
  .close-btn:hover { color: var(--text); }

  .hist-body {
    display: flex;
    flex: 1;
    overflow: hidden;
  }

  .hist-list {
    width: 240px;
    min-width: 200px;
    border-right: 1px solid var(--border);
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0;
  }

  .version-item {
    display: flex;
    align-items: flex-start;
    gap: .5rem;
    padding: .6rem .75rem;
    border: none;
    background: none;
    cursor: pointer;
    text-align: left;
    border-bottom: 1px solid var(--border);
    color: var(--text);
    transition: background .12s;
  }
  .version-item:hover { background: var(--bg-hover); }
  .version-item.active { background: var(--bg-active, var(--bg-hover)); }

  .version-icon { font-size: 1rem; color: var(--text-muted); margin-top: .1rem; flex-shrink: 0; }

  .version-info {
    display: flex;
    flex-direction: column;
    gap: .1rem;
    min-width: 0;
  }
  .version-title {
    font-size: .8rem;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .version-date {
    font-size: .7rem;
    color: var(--text-muted);
  }

  .version-badge {
    margin-left: auto;
    font-size: .65rem;
    padding: .1rem .35rem;
    border-radius: 4px;
    background: var(--accent);
    color: #fff;
    flex-shrink: 0;
    align-self: center;
  }

  .hist-preview {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
  }

  .preview-header {
    padding: .5rem 1rem;
    border-bottom: 1px solid var(--border);
    background: var(--bg-surface, var(--bg));
    position: sticky;
    top: 0;
    z-index: 1;
  }
  .preview-meta { font-size: .75rem; color: var(--text-muted); }

  .preview-content {
    padding: 1rem 1.25rem;
    flex: 1;
    font-size: .9rem;
    line-height: 1.6;
    pointer-events: none;
  }

  .hist-empty {
    color: var(--text-muted);
    font-size: .875rem;
    padding: 1rem;
    text-align: center;
  }

  .hist-footer {
    display: flex;
    align-items: center;
    gap: .5rem;
    justify-content: flex-end;
    padding: .75rem 1.25rem;
    border-top: 1px solid var(--border);
  }

  .confirm-text {
    font-size: .85rem;
    color: var(--text);
    margin-right: auto;
  }

  @media (max-width: 600px) {
    .hist-modal { width: 98vw; height: 92vh; }
    .hist-list { width: 140px; min-width: 120px; }
    .version-title { font-size: .75rem; }
  }
</style>
