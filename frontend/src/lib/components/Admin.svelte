<script>
  import { onMount } from 'svelte'
  import { apiGet, apiPost, apiDelete, apiFetch } from '../api.js'
  import { loadTags, tags } from '../stores/tags.js'

  let trash = $state([])
  let renameOld = $state('')
  let renameNew = $state('')
  let renameMsg = $state('')
  let cleanupResult = $state(null)
  let loading = $state(false)
  let activeTab = $state('backup')

  onMount(() => {
    loadTags()
    loadTrash()
  })

  async function loadTrash() {
    trash = await apiGet('/api/admin/trash')
  }

  // Backup
  async function handleBackup() {
    loading = true
    try {
      const res = await apiFetch('/api/admin/backup', { method: 'POST' })
      if (!res.ok) { alert('Erro ao fazer backup'); return }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `pkd-backup-${new Date().toISOString().slice(0,10)}.sqlite`
      a.click()
      URL.revokeObjectURL(url)
    } finally {
      loading = false
    }
  }

  // Restore
  let restoreFile = $state(null)
  async function handleRestore() {
    if (!restoreFile || !confirm('ATENÇÃO: Isso irá substituir todos os dados atuais. Continuar?')) return
    loading = true
    try {
      const fd = new FormData()
      fd.append('file', restoreFile)
      fd.append('confirm', 'REPLACE')
      const res = await apiFetch('/api/admin/restore', { method: 'POST', body: fd })
      if (res.ok) {
        alert('Restauração concluída. Recarregando…')
        window.location.reload()
      } else {
        alert('Erro na restauração: ' + await res.text())
      }
    } finally {
      loading = false
    }
  }

  // Trash management
  async function restoreFromTrash(id) {
    await apiPost(`/api/documents/${id}/restore`)
    await loadTrash()
  }

  async function permanentDelete(id) {
    if (!confirm('Excluir permanentemente? Esta ação não pode ser desfeita.')) return
    await apiDelete(`/api/admin/trash/${id}`)
    await loadTrash()
  }

  async function emptyTrash() {
    if (!confirm('Esvaziar toda a lixeira? Esta ação não pode ser desfeita.')) return
    await apiPost('/api/admin/trash/empty')
    await loadTrash()
  }

  // Tag rename/merge
  async function handleRenameTag() {
    renameMsg = ''
    if (!renameOld || !renameNew) { renameMsg = 'Preencha os dois campos.'; return }
    const res = await apiFetch('/api/admin/tags/rename', {
      method: 'PUT',
      body: JSON.stringify({ old: renameOld, new: renameNew }),
    })
    if (res.ok) {
      renameMsg = `Tag "#${renameOld}" renomeada para "#${renameNew}".`
      renameOld = ''
      renameNew = ''
      await loadTags()
    } else {
      renameMsg = 'Erro: ' + await res.text()
    }
  }

  // Cleanup
  async function handleCleanup() {
    loading = true
    try {
      const data = await apiPost('/api/admin/cleanup')
      cleanupResult = data
    } finally {
      loading = false
    }
  }
</script>

<div class="admin-wrap">
  <h1 class="admin-title">⚙️ Administração</h1>

  <!-- Tabs -->
  <div class="tabs">
    {#each [['backup','💾 Backup'], ['trash','🗑️ Lixeira'], ['tags','🏷️ Tags'], ['cleanup','🧹 Limpeza']] as [id, label]}
      <button
        class="tab-btn {activeTab === id ? 'active' : ''}"
        onclick={() => activeTab = id}
      >{label}</button>
    {/each}
  </div>

  <!-- Backup / Restore -->
  {#if activeTab === 'backup'}
    <div class="admin-section">
      <h3>Backup</h3>
      <button class="btn btn-primary" onclick={handleBackup} disabled={loading}>
        ⬇ Fazer download do backup
      </button>
    </div>
    <div class="admin-section">
      <h3>Restaurar</h3>
      <p class="muted" style="margin-bottom:.75rem">Selecione um arquivo .sqlite de backup.</p>
      <input type="file" accept=".sqlite,.db" onchange={e => restoreFile = e.target.files?.[0]} />
      <button class="btn btn-danger" style="margin-top:.75rem" onclick={handleRestore} disabled={loading || !restoreFile}>
        ⚠ Restaurar (substitui todos os dados)
      </button>
    </div>
  {/if}

  <!-- Trash -->
  {#if activeTab === 'trash'}
    <div class="admin-section">
      <div class="section-header">
        <h3>Lixeira ({trash.length})</h3>
        {#if trash.length > 0}
          <button class="btn btn-danger" onclick={emptyTrash}>Esvaziar tudo</button>
        {/if}
      </div>

      {#if trash.length === 0}
        <p class="muted">Lixeira vazia.</p>
      {:else}
        {#each trash as doc}
          <div class="trash-item">
            <span class="trash-icon">{doc.icon || '📄'}</span>
            <span class="trash-title">{doc.title}</span>
            <span class="trash-date">{new Date(doc.trashed_at).toLocaleDateString('pt-BR')}</span>
            <button class="btn btn-ghost btn-sm" onclick={() => restoreFromTrash(doc.id)}>Restaurar</button>
            <button class="btn btn-danger btn-sm" onclick={() => permanentDelete(doc.id)}>Excluir</button>
          </div>
        {/each}
      {/if}
    </div>
  {/if}

  <!-- Tags -->
  {#if activeTab === 'tags'}
    <div class="admin-section">
      <h3>Renomear / Mesclar tags</h3>
      <div class="rename-row">
        <input bind:value={renameOld} placeholder="Tag atual" list="tags-list" />
        <span>→</span>
        <input bind:value={renameNew} placeholder="Novo nome" />
        <button class="btn btn-primary" onclick={handleRenameTag}>Renomear</button>
      </div>
      {#if renameMsg}<p class="rename-msg">{renameMsg}</p>{/if}
      <datalist id="tags-list">
        {#each $tags as tag}<option value={tag.name}>{/each}
      </datalist>
    </div>
    <div class="admin-section">
      <h3>Todas as tags</h3>
      <div class="tag-list">
        {#each $tags as tag}
          <span class="tag-chip"># {tag.name} <small>({tag.count})</small></span>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Cleanup -->
  {#if activeTab === 'cleanup'}
    <div class="admin-section">
      <h3>Limpeza</h3>
      <p class="muted" style="margin-bottom:.75rem">Remove anexos órfãos e executa VACUUM no banco.</p>
      <button class="btn btn-primary" onclick={handleCleanup} disabled={loading}>
        {loading ? 'Limpando…' : '🧹 Iniciar limpeza'}
      </button>
      {#if cleanupResult}
        <p class="cleanup-result">✓ {cleanupResult.orphans_removed} anexos órfãos removidos.</p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .admin-wrap {
    padding: 1.5rem 2rem;
    max-width: 700px;
    margin: 0 auto;
    width: 100%;
  }

  .admin-title { font-size: 1.25rem; font-weight: 700; margin-bottom: 1.25rem; }

  .tabs {
    display: flex;
    gap: .25rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 1.25rem;
  }

  .tab-btn {
    padding: .5rem .875rem;
    border-radius: var(--radius) var(--radius) 0 0;
    font-size: .875rem;
    color: var(--text-muted);
    cursor: pointer;
    border-bottom: 2px solid transparent;
  }
  .tab-btn:hover { color: var(--text); }
  .tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); font-weight: 500; }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: .75rem;
  }

  .trash-item {
    display: flex;
    align-items: center;
    gap: .5rem;
    padding: .4rem 0;
    border-bottom: 1px solid var(--border);
    font-size: .9rem;
  }
  .trash-icon { flex-shrink: 0; }
  .trash-title { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .trash-date { color: var(--text-muted); font-size: .8rem; flex-shrink: 0; }

  .rename-row {
    display: flex;
    align-items: center;
    gap: .5rem;
    flex-wrap: wrap;
  }
  .rename-row input { flex: 1; min-width: 120px; padding: .45rem .75rem; }
  .rename-msg { margin-top: .5rem; font-size: .875rem; color: var(--success); }

  .tag-list {
    display: flex;
    flex-wrap: wrap;
    gap: .375rem;
    margin-top: .5rem;
  }

  .cleanup-result {
    margin-top: .75rem;
    color: var(--success);
    font-size: .9rem;
  }

  .btn-sm { padding: .25rem .6rem; font-size: .8rem; }
  .muted { color: var(--text-muted); font-size: .875rem; }
</style>
