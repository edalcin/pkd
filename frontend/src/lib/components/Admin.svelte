<script>
  import { onMount } from 'svelte'
  import { apiGet, apiPost, apiPut, apiDelete, apiFetch } from '../api.js'
  import { loadTags, tags } from '../stores/tags.js'
  import { autoSaveInterval } from '../stores/settings.js'
  import { docBodyRefreshedSignal } from '../stores/documents.js'

  const AUTOSAVE_OPTIONS = [
    { value: 5000,  label: '5 segundos' },
    { value: 15000, label: '15 segundos' },
    { value: 30000, label: '30 segundos' },
    { value: 0,     label: 'Desligado' },
  ]

  let trash = $state([])
  let renameOld = $state('')
  let renameNew = $state('')
  let renameMsg = $state('')
  let cleanupResult = $state(null)
  let loading = $state(false)
  let activeTab = $state('backup')
  let linkCheckResults = $state(null)
  let linkChecking = $state(false)

  // Attachments tab
  let allAttachments = $state([])
  let allOrphans = $state([])
  let orphansLoading = $state(false)
  let attachmentsLoading = $state(false)
  let previewAtt = $state(null)

  // External images tab
  let externalImagesDocs = $state([])
  let externalImagesLoading = $state(false)
  let externalImagesImporting = $state({}) // { [docId]: true }
  let externalImagesMsg = $state({}) // { [docId]: string }

  // Settings tab — version history
  let versionsMaxPerDoc = $state('50')
  let versionsSettingMsg = $state('')
  let versionsSettingSaving = $state(false)

  // Tags tab — local editable copy
  let editableTags = $state([])
  let tagMsg = $state('')
  let pruneMsg = $state('')

  // Shares tab
  let shares = $state([])
  let sharesLoading = $state(false)

  // Storage tab
  let storageConfig = $state(null)
  let storageTestResult = $state(null)
  let storageMigrateResult = $state(null)
  let storageCleanupResult = $state(null)
  let storageReconcileResult = $state(null)
  let storageLoading = $state(false)
  let storageSwitching = $state(false)
  let storageRestoreResult = $state(null)
  let storageRestoreInput = $state(null)

  // Async S3 backup job (005-s3-attachments-backup)
  let s3BackupJob = $state(null) // { id, state, processed, total, download_url, url_expires_at, error_message, size_bytes }
  let s3BackupPolling = $state(false)
  let s3BackupStarting = $state(false)
  let s3BackupCountdown = $state(0) // seconds remaining on the presigned URL

  // Async restore job (US2/US3 — cross-backend and in-place restore)
  let restoreJob = $state(null) // { id, state, processed, total, error_message, restore: {written, kept, skipped_orphan, hash_mismatch, skipped[]} }
  let restorePolling = $state(false)
  let restoreStarting = $state(false)
  let restoreZipInput = $state(null)
  let restoreOnConflict = $state('overwrite')
  let restoreShowSkipped = $state(false)

  // Links management tab
  let adminURLs = $state(null)
  let urlSearch = $state('')
  let urlSortDir = $state('asc')
  let urlEditId = $state(null)
  let urlEditForm = $state({ url: '', title: '' })
  let urlSaving = $state(false)

  let filteredURLs = $derived.by(() => {
    const list = adminURLs ?? []
    const q = urlSearch.trim().toLowerCase()
    const filtered = q ? list.filter(u => u.document_title.toLowerCase().includes(q)) : list
    return [...filtered].sort((a, b) => {
      const cmp = a.document_title.localeCompare(b.document_title, 'pt-BR', { sensitivity: 'base' })
      return urlSortDir === 'asc' ? cmp : -cmp
    })
  })

  onMount(() => {
    loadTags()
    loadTrash()
    apiGet('/api/admin/settings').then(data => {
      if (data?.['versions.max_per_doc']) versionsMaxPerDoc = data['versions.max_per_doc']
    })
  })

  async function loadTrash() {
    trash = (await apiGet('/api/admin/trash')) || []
  }

  async function saveVersionsSettings() {
    const n = parseInt(versionsMaxPerDoc, 10)
    if (!n || n < 1 || n > 10000) {
      versionsSettingMsg = 'Valor inválido. Use um número entre 1 e 10000.'
      return
    }
    versionsSettingSaving = true
    versionsSettingMsg = ''
    try {
      await apiPut('/api/admin/settings', { key: 'versions.max_per_doc', value: String(n) })
      versionsSettingMsg = 'Salvo!'
      setTimeout(() => { versionsSettingMsg = '' }, 2000)
    } catch {
      versionsSettingMsg = 'Erro ao salvar.'
    } finally {
      versionsSettingSaving = false
    }
  }

  async function loadAttachments() {
    attachmentsLoading = true
    try {
      allAttachments = (await apiGet('/api/admin/attachments')) || []
    } finally {
      attachmentsLoading = false
    }
  }

  async function loadOrphans() {
    orphansLoading = true
    try {
      allOrphans = (await apiGet('/api/admin/attachments/orphans')) || []
    } finally {
      orphansLoading = false
    }
  }

  async function loadExternalImages() {
    externalImagesLoading = true
    try {
      externalImagesDocs = (await apiGet('/api/admin/external-images')) || []
    } finally {
      externalImagesLoading = false
    }
  }

  async function importDocExternalImages(docId) {
    externalImagesImporting = { ...externalImagesImporting, [docId]: true }
    externalImagesMsg = { ...externalImagesMsg, [docId]: '' }
    try {
      const res = await apiFetch(`/api/admin/documents/${docId}/import-external-images`, { method: 'POST' })
      if (!res.ok) {
        externalImagesMsg = { ...externalImagesMsg, [docId]: 'Erro ao importar' }
        return
      }
      const data = await res.json()
      externalImagesMsg = { ...externalImagesMsg, [docId]: `${data.imported} importada(s)${data.failed ? ', ' + data.failed + ' falhou' : ''}` }
      docBodyRefreshedSignal.set(docId) // signal editor to reload if doc is open
      await loadExternalImages()
      await loadAttachments()
    } finally {
      externalImagesImporting = { ...externalImagesImporting, [docId]: false }
    }
  }

  async function downloadOrphan(key) {
    const res = await apiFetch(`/api/admin/attachments/orphans/download?key=${encodeURIComponent(key)}`)
    if (!res.ok) { alert('Erro ao baixar arquivo'); return }
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = key.split('/').pop()
    a.click()
  }

  async function deleteOrphan(orphan) {
    const name = orphan.original_name || orphan.key.split('/').pop()
    let msg = `Eliminar "${name}"? Esta ação não pode ser desfeita.`
    if (orphan.reason === 'trashed_doc' && orphan.doc_title) {
      msg = `Eliminar "${name}"?\n\nO documento na lixeira "${orphan.doc_title}" também será excluído permanentemente.\n\nEsta ação não pode ser desfeita.`
    }
    if (!confirm(msg)) return
    await apiFetch(`/api/admin/attachments/orphans/item?key=${encodeURIComponent(orphan.key)}`, { method: 'DELETE' })
    allOrphans = allOrphans.filter(o => o.key !== orphan.key)
  }

  let deletingAllOrphans = $state(false)

  async function deleteAllOrphans() {
    const hasTrashDocs = allOrphans.some(o => o.reason === 'trashed_doc')
    let msg = `Eliminar todos os ${allOrphans.length} arquivos órfãos? Esta ação não pode ser desfeita.`
    if (hasTrashDocs) msg += '\n\nDocumentos na lixeira associados também serão excluídos permanentemente.'
    if (!confirm(msg)) return
    deletingAllOrphans = true
    try {
      await apiFetch('/api/admin/attachments/orphans', { method: 'DELETE' })
      allOrphans = []
    } finally {
      deletingAllOrphans = false
    }
  }

  function previewOrphan(orphan) {
    const name = orphan.original_name || orphan.key.split('/').pop()
    // Type 2 (has attachment_id): use the standard attachment endpoint (inline, correct MIME).
    // Type 1 (no DB record): use the admin download endpoint (images render fine in <img> tag).
    const url = orphan.attachment_id
      ? `/api/attachments/${orphan.attachment_id}`
      : `/api/admin/attachments/orphans/download?key=${encodeURIComponent(orphan.key)}`
    previewAtt = { original_name: name, url, mime_type: orphan.mime_type || '', size_bytes: orphan.size_bytes }
  }

  function setTab(id) {
    activeTab = id
    if (id === 'attachments' && allAttachments.length === 0) { loadAttachments(); loadOrphans(); loadExternalImages() }
    if (id === 'tags') loadAdminTags()
    if (id === 'shares') loadShares()
    if (id === 'links') loadAdminURLs()
    if (id === 'storage') loadStorageConfig()
  }

  async function loadStorageConfig() {
    storageConfig = await apiGet('/api/admin/storage/config')
  }

  async function switchStorageBackend(backend) {
    storageSwitching = true
    try {
      await apiPut('/api/admin/storage/config', { backend })
      await loadStorageConfig()
    } finally {
      storageSwitching = false
    }
  }

  async function testStorage() {
    storageLoading = true
    storageTestResult = null
    try {
      storageTestResult = await apiPost('/api/admin/storage/test', {})
    } finally {
      storageLoading = false
    }
  }

  async function migrateStorage() {
    if (!confirm('Migrar todos os arquivos para o backend ativo? Operação pode demorar alguns segundos.')) return
    storageLoading = true
    storageMigrateResult = null
    try {
      storageMigrateResult = await apiPost('/api/admin/storage/migrate', {})
    } finally {
      storageLoading = false
    }
  }

  async function reconcileStorage() {
    if (!confirm('Verificar o bucket S3 e corrigir registros com storage_location incorreto? Isso atualiza apenas o banco — não move arquivos.')) return
    storageLoading = true
    storageReconcileResult = null
    try {
      storageReconcileResult = await apiPost('/api/admin/storage/reconcile', {})
    } finally {
      storageLoading = false
    }
  }

  async function backupAttachments() {
    const res = await apiFetch('/api/admin/storage/backup-attachments')
    if (!res.ok) { alert('Erro ao gerar backup'); return }
    const blob = await res.blob()
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `pkd-attachments-${new Date().toISOString().slice(0,10)}.zip`
    a.click()
  }

  // ---- Async S3 backup (005-s3-attachments-backup) ----

  async function startS3Backup() {
    if (s3BackupJob && s3BackupJob.state === 'running') return
    s3BackupStarting = true
    s3BackupJob = null
    try {
      const res = await apiFetch('/api/admin/storage/backup-start', { method: 'POST' })
      if (res.status === 409) {
        alert('Já existe uma operação em andamento para este backend.')
        return
      }
      if (!res.ok) {
        const msg = await res.text()
        alert('Erro ao iniciar backup: ' + msg)
        return
      }
      const { job_id } = await res.json()
      s3BackupJob = { id: job_id, state: 'running', processed: 0, total: 0 }
      pollS3BackupJob()
    } finally {
      s3BackupStarting = false
    }
  }

  async function pollS3BackupJob() {
    if (!s3BackupJob?.id || s3BackupPolling) return
    s3BackupPolling = true
    try {
      while (s3BackupJob && s3BackupJob.state === 'running') {
        await new Promise(r => setTimeout(r, 2000))
        const res = await apiFetch(`/api/admin/storage/jobs/${s3BackupJob.id}`)
        if (!res.ok) {
          s3BackupJob = { ...s3BackupJob, state: 'failed', error_message: 'Falha ao consultar status' }
          break
        }
        s3BackupJob = await res.json()
      }
      if (s3BackupJob?.state === 'succeeded') {
        startCountdown()
      }
    } finally {
      s3BackupPolling = false
    }
  }

  function startCountdown() {
    if (!s3BackupJob?.url_expires_at) return
    const tick = () => {
      const remaining = Math.max(0, Math.floor((new Date(s3BackupJob.url_expires_at) - Date.now()) / 1000))
      s3BackupCountdown = remaining
      if (remaining > 0 && s3BackupJob?.state === 'succeeded') {
        setTimeout(tick, 1000)
      }
    }
    tick()
  }

  async function regenerateBackupURL() {
    if (!s3BackupJob?.id) return
    const res = await apiFetch(`/api/admin/storage/jobs/${s3BackupJob.id}/download-url`, { method: 'POST' })
    if (res.status === 404) {
      alert('ZIP temporário expirou no S3. Inicie um novo backup.')
      s3BackupJob = null
      return
    }
    if (!res.ok) {
      alert('Falha ao gerar nova URL.')
      return
    }
    const { download_url, url_expires_at } = await res.json()
    s3BackupJob = { ...s3BackupJob, download_url, url_expires_at }
    startCountdown()
  }

  function formatCountdown(s) {
    const m = Math.floor(s / 60)
    const r = s % 60
    return `${m}:${String(r).padStart(2, '0')}`
  }

  // ---- Async restore (US2/US3) ----

  async function startRestore() {
    if (!restoreZipInput?.files?.[0]) { alert('Selecione um arquivo ZIP gerado por esta aplicação.'); return }
    if (restoreJob && restoreJob.state === 'running') return
    restoreStarting = true
    restoreJob = null
    restoreShowSkipped = false
    try {
      const fd = new FormData()
      fd.append('zip', restoreZipInput.files[0])
      fd.append('on_conflict', restoreOnConflict)
      const res = await apiFetch('/api/admin/storage/restore-start', { method: 'POST', body: fd })
      if (res.status === 409) {
        alert('Já existe uma operação em andamento para este backend.')
        return
      }
      if (!res.ok) {
        const msg = await res.text()
        alert('Erro ao iniciar restauração: ' + msg)
        return
      }
      const { job_id } = await res.json()
      restoreJob = { id: job_id, state: 'running', processed: 0, total: 0 }
      pollRestoreJob()
    } finally {
      restoreStarting = false
    }
  }

  async function pollRestoreJob() {
    if (!restoreJob?.id || restorePolling) return
    restorePolling = true
    try {
      while (restoreJob && restoreJob.state === 'running') {
        await new Promise(r => setTimeout(r, 2000))
        const res = await apiFetch(`/api/admin/storage/jobs/${restoreJob.id}`)
        if (!res.ok) {
          restoreJob = { ...restoreJob, state: 'failed', error_message: 'Falha ao consultar status' }
          break
        }
        restoreJob = await res.json()
      }
    } finally {
      restorePolling = false
    }
  }

  async function restoreAttachments() {
    if (!storageRestoreInput?.files?.[0]) { alert('Selecione um arquivo ZIP'); return }
    storageLoading = true
    storageRestoreResult = null
    try {
      const fd = new FormData()
      fd.append('file', storageRestoreInput.files[0])
      const res = await apiFetch('/api/admin/storage/restore-attachments', { method: 'POST', body: fd })
      storageRestoreResult = await res.json()
    } finally {
      storageLoading = false
    }
  }

  async function cleanupStorageSource() {
    if (!confirm('Remover arquivos da origem após migração confirmada? Esta ação não pode ser desfeita.')) return
    storageLoading = true
    storageCleanupResult = null
    try {
      storageCleanupResult = await apiPost('/api/admin/storage/cleanup-source', {})
    } finally {
      storageLoading = false
    }
  }

  async function loadShares() {
    sharesLoading = true
    try {
      shares = (await apiGet('/api/admin/shares')) || []
    } finally {
      sharesLoading = false
    }
  }

  async function revokeShare(id) {
    if (!confirm('Revogar este link de compartilhamento? Ele deixará de funcionar imediatamente.')) return
    await apiDelete(`/api/admin/shares/${id}`)
    shares = shares.filter(s => s.id !== id)
  }

  let copyFeedback = $state('')
  async function copyShareURL(url) {
    try {
      await navigator.clipboard.writeText(url)
      copyFeedback = url
      setTimeout(() => { copyFeedback = '' }, 2000)
    } catch {
      prompt('Copie o link:', url)
    }
  }

  async function loadAdminTags() {
    const all = (await apiGet('/api/admin/tags')) || []
    editableTags = all.map(t => ({ ...t, _editing: false, _editName: t.name, _editColor: t.color || '#6b7280' }))
  }

  function syncEditableTags() {
    loadAdminTags()
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

  // Tag management
  async function handleRenameTag() {
    renameMsg = ''
    if (!renameOld || !renameNew) { renameMsg = 'Preencha os dois campos.'; return }
    const res = await apiFetch('/api/admin/tags/rename', {
      method: 'PUT',
      body: JSON.stringify({ old: renameOld, new: renameNew }),
    })
    if (res.ok) {
      renameMsg = `Tag "#${renameOld}" mesclada/renomeada para "#${renameNew}".`
      renameOld = ''
      renameNew = ''
      await loadTags()
      syncEditableTags()
    } else {
      renameMsg = 'Erro: ' + await res.text()
    }
  }

  function startEdit(tag) {
    tag._editing = true
    tag._editName = tag.name
    tag._editColor = tag.color || '#6b7280'
    editableTags = [...editableTags]
  }

  function cancelEdit(tag) {
    tag._editing = false
    editableTags = [...editableTags]
  }

  async function saveTag(tag) {
    tagMsg = ''
    const res = await apiFetch(`/api/admin/tags/${tag.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: tag._editName, color: tag._editColor }),
    })
    if (res.ok) {
      tag._editing = false
      await loadTags()
      syncEditableTags()
      tagMsg = 'Tag salva.'
    } else {
      tagMsg = 'Erro ao salvar: ' + await res.text()
    }
  }

  async function deleteTag(tag) {
    if (!confirm(`Excluir a tag "#${tag.name}"? Ela será removida de todos os documentos.`)) return
    tagMsg = ''
    const res = await apiFetch(`/api/admin/tags/${tag.id}`, { method: 'DELETE' })
    if (res.ok) {
      await loadTags()
      syncEditableTags()
      tagMsg = `Tag "#${tag.name}" excluída.`
    } else {
      tagMsg = 'Erro ao excluir: ' + await res.text()
    }
  }

  async function handlePruneTags() {
    const orphans = editableTags.filter(t => t.count === 0)
    if (orphans.length === 0) { pruneMsg = 'Nenhuma tag órfã encontrada.'; return }
    if (!confirm(`Remover ${orphans.length} tag(s) sem documentos?`)) return
    pruneMsg = ''
    const res = await apiFetch('/api/admin/tags/prune', { method: 'POST' })
    if (res.ok) {
      const data = await res.json()
      pruneMsg = `${data.removed} tag(s) órfã(s) removida(s).`
      await loadTags()
      syncEditableTags()
    } else {
      pruneMsg = 'Erro: ' + await res.text()
    }
  }

  async function saveTagColor(tag) {
    await apiFetch(`/api/admin/tags/${tag.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: tag.name, color: tag._editColor }),
    })
    await loadTags()
    syncEditableTags()
  }

  // Attachment management
  async function deleteAttachment(att) {
    if (!confirm(`Excluir o arquivo "${att.original_name}"?`)) return
    await apiDelete(`/api/attachments/${att.id}`)
    allAttachments = allAttachments.filter(a => a.id !== att.id)
  }

  // Cleanup tab
  async function handleCleanup() {
    loading = true
    try {
      cleanupResult = await apiPost('/api/admin/cleanup')
    } finally {
      loading = false
    }
  }

  // URL validity check
  async function handleCheckLinks() {
    linkChecking = true
    linkCheckResults = null
    try {
      linkCheckResults = await apiPost('/api/admin/check-urls')
    } finally {
      linkChecking = false
    }
  }

  async function deleteInvalidLinks() {
    if (!linkCheckResults) return
    const invalid = linkCheckResults.filter(r => !r.valid)
    if (!invalid.length) return
    if (!confirm(`Excluir ${invalid.length} link(s) inválido(s)?`)) return
    for (const r of invalid) {
      await apiDelete(`/api/documents/${r.document_id}/urls/${r.id}`)
    }
    linkCheckResults = linkCheckResults.filter(r => r.valid)
  }

  async function loadAdminURLs() {
    try {
      adminURLs = await apiGet('/api/admin/urls')
    } catch {
      adminURLs = []
    }
  }

  function startEditURL(u) {
    urlEditId = u.id
    urlEditForm = { url: u.url, title: u.title }
  }

  function cancelEditURL() {
    urlEditId = null
  }

  async function saveEditURL(u) {
    if (!urlEditForm.url.trim()) return
    urlSaving = true
    try {
      await apiPut(`/api/documents/${u.document_id}/urls/${u.id}`, urlEditForm)
      adminURLs = adminURLs.map(x => x.id === u.id ? { ...x, ...urlEditForm } : x)
      urlEditId = null
    } finally {
      urlSaving = false
    }
  }

  async function deleteAdminURL(u) {
    if (!confirm('Excluir este link?')) return
    await apiDelete(`/api/documents/${u.document_id}/urls/${u.id}`)
    adminURLs = adminURLs.filter(x => x.id !== u.id)
  }

  function isImage(mime) {
    return mime && mime.startsWith('image/')
  }

  function previewType(mime) {
    if (!mime) return 'download'
    if (mime.startsWith('image/')) return 'image'
    if (mime === 'application/pdf') return 'pdf'
    if (mime.startsWith('audio/')) return 'audio'
    if (mime.startsWith('video/')) return 'video'
    return 'download'
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  }

  function fileIcon(mime) {
    if (!mime) return '📎'
    if (mime.startsWith('image/')) return '🖼️'
    if (mime.startsWith('video/')) return '🎬'
    if (mime.startsWith('audio/')) return '🎵'
    if (mime.includes('pdf')) return '📄'
    if (mime.includes('zip') || mime.includes('archive')) return '🗜️'
    if (mime.includes('spreadsheet') || mime.includes('excel')) return '📊'
    if (mime.includes('document') || mime.includes('word')) return '📝'
    return '📎'
  }
</script>

<div class="admin-wrap" class:admin-wrap-wide={activeTab === 'links'}>
  <h1 class="admin-title">⚙️ Administração</h1>

  <!-- Tabs -->
  <div class="tabs">
    {#each [['backup','💾 Backup'], ['trash','🗑️ Lixeira'], ['tags','🏷️ Tags'], ['attachments','📎 Arquivos'], ['cleanup','🧹 Limpeza'], ['links','🔗 Links'], ['shares','🌐 Compartilhados'], ['storage','☁️ Storage'], ['settings','⚙️ Preferências']] as [id, label]}
      <button
        class="tab-btn {activeTab === id ? 'active' : ''}"
        onclick={() => setTab(id)}
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
            <i class="bx {doc.icon || 'bx-file-blank'} trash-icon"></i>
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
      <div class="section-header">
        <h3>Tags ({editableTags.length})</h3>
        <div style="display:flex;align-items:center;gap:.5rem;flex-wrap:wrap">
          {#if tagMsg}<span class="tag-msg">{tagMsg}</span>{/if}
          {#if pruneMsg}<span class="tag-msg">{pruneMsg}</span>{/if}
          {#if editableTags.filter(t => t.count === 0).length > 0}
            <button class="btn btn-danger btn-sm" onclick={handlePruneTags}>
              🧹 Remover {editableTags.filter(t => t.count === 0).length} tag(s) órfã(s)
            </button>
          {/if}
        </div>
      </div>

      {#if editableTags.length === 0}
        <p class="muted">Nenhuma tag cadastrada.</p>
      {:else}
        <div class="tag-table">
          {#each editableTags as tag}
            <div class="tag-row">
              {#if tag._editing}
                <input
                  type="color"
                  class="color-input"
                  bind:value={tag._editColor}
                  title="Cor da tag"
                />
                <input
                  class="tag-name-input"
                  bind:value={tag._editName}
                  onkeydown={e => e.key === 'Enter' && saveTag(tag)}
                />
                <span class="tag-count muted">({tag.count})</span>
                <button class="btn btn-primary btn-sm" onclick={() => saveTag(tag)}>Salvar</button>
                <button class="btn btn-ghost btn-sm" onclick={() => cancelEdit(tag)}>Cancelar</button>
              {:else}
                <span
                  class="tag-color-dot"
                  style="background:{tag.color || '#6b7280'}"
                  title={tag.color || 'sem cor'}
                ></span>
                <span class="tag-name"># {tag.name}</span>
                <span class="tag-count muted">({tag.count})</span>
                <button class="btn btn-ghost btn-sm" onclick={() => startEdit(tag)} title="Editar">✏️</button>
                <button class="btn btn-danger btn-sm" onclick={() => deleteTag(tag)} title="Excluir">🗑</button>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <div class="admin-section">
      <h3>Mesclar tags</h3>
      <p class="muted" style="margin-bottom:.5rem">Renomeia uma tag ou mescla-a em outra existente.</p>
      <div class="rename-row">
        <input bind:value={renameOld} placeholder="Tag de origem" list="tags-list" />
        <span>→</span>
        <input bind:value={renameNew} placeholder="Tag de destino" />
        <button class="btn btn-primary" onclick={handleRenameTag}>Mesclar / Renomear</button>
      </div>
      {#if renameMsg}<p class="rename-msg">{renameMsg}</p>{/if}
      <datalist id="tags-list">
        {#each $tags as tag}<option value={tag.name}></option>{/each}
      </datalist>
    </div>
  {/if}

  <!-- Attachments -->
  {#if activeTab === 'attachments'}
    <!-- External images sub-section -->
    {#if externalImagesDocs.length > 0 || externalImagesLoading}
      <div class="admin-section">
        <div class="section-header">
          <h3>Imagens externas{externalImagesDocs.length > 0 ? ` (${externalImagesDocs.length} doc.)` : ''}</h3>
          <button class="btn btn-ghost btn-sm" onclick={loadExternalImages} disabled={externalImagesLoading}>
            {externalImagesLoading ? 'Verificando…' : '↻ Atualizar'}
          </button>
        </div>
        <p class="muted" style="margin-bottom:.75rem">
          Documentos com imagens hospedadas externamente (copiadas de outras páginas). Importar converte cada imagem em um anexo interno.
        </p>
        {#if externalImagesLoading}
          <p class="muted">Verificando…</p>
        {:else}
          <div class="orphan-list">
            {#each externalImagesDocs as item}
              <div class="orphan-row">
                <div class="orphan-info">
                  <a href="#/doc/{item.doc_id}" class="orphan-name">{item.doc_title}</a>
                  <span class="orphan-doc muted">{item.count} imagem(ns) externa(s)</span>
                </div>
                {#if externalImagesMsg[item.doc_id]}
                  <span class="muted" style="font-size:.85em">{externalImagesMsg[item.doc_id]}</span>
                {/if}
                <button class="btn btn-primary btn-sm"
                  onclick={() => importDocExternalImages(item.doc_id)}
                  disabled={externalImagesImporting[item.doc_id]}>
                  {externalImagesImporting[item.doc_id] ? '⏳ Importando…' : '🌐⬇ Importar'}
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {#if allOrphans.length > 0 || orphansLoading}
      <div class="admin-section orphan-section">
        <div class="section-header">
          <h3>Arquivos órfãos{allOrphans.length > 0 ? ` (${allOrphans.length})` : ''}</h3>
          {#if allOrphans.length > 1}
            <button class="btn btn-danger btn-sm" onclick={deleteAllOrphans} disabled={deletingAllOrphans}>
              {deletingAllOrphans ? 'Eliminando…' : '🗑 Eliminar todos'}
            </button>
          {/if}
        </div>
        {#if orphansLoading}
          <p class="muted">Verificando…</p>
        {:else if allOrphans.length === 0}
          <p class="muted">Nenhum arquivo órfão encontrado.</p>
        {:else}
          <p class="muted" style="margin-bottom:.75rem">
            Arquivos no disco sem registro no banco de dados. Podem ser resquícios de uploads com falha.
          </p>
          <div class="orphan-list">
            {#each allOrphans as orphan}
              <div class="orphan-row">
                <div class="orphan-info">
                  <button class="orphan-name orphan-name-btn" title="Clique para visualizar" onclick={() => previewOrphan(orphan)}>
                    {orphan.original_name || orphan.key.split('/').pop()}
                  </button>
                  {#if orphan.reason === 'trashed_doc' && orphan.doc_title}
                    <span class="orphan-doc muted">🗑 Doc na lixeira: "{orphan.doc_title}"</span>
                  {:else if orphan.reason === 'no_doc'}
                    <span class="orphan-doc muted">Documento não encontrado</span>
                  {/if}
                </div>
                <span class="orphan-reason muted">{orphan.reason === 'trashed_doc' ? 'lixeira' : orphan.reason === 'no_doc' ? 'sem doc' : 'sem registro'}</span>
                <span class="orphan-size muted">{formatBytes(orphan.size_bytes)}</span>
                <button class="btn btn-ghost btn-sm" onclick={() => downloadOrphan(orphan.key)}>⬇ Baixar</button>
                <button class="btn btn-danger btn-sm" onclick={() => deleteOrphan(orphan)}>🗑 Eliminar</button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    <div class="admin-section">
      <div class="section-header">
        <h3>Arquivos anexados ({allAttachments.length})</h3>
        <div style="display:flex;gap:.5rem">
          <button class="btn btn-ghost btn-sm" onclick={loadAttachments} disabled={attachmentsLoading}>
            {attachmentsLoading ? 'Carregando…' : '↻ Atualizar'}
          </button>
          <button class="btn btn-ghost btn-sm" onclick={loadOrphans} disabled={orphansLoading}>
            {orphansLoading ? 'Verificando…' : '🔍 Verificar Órfãos'}
          </button>
        </div>
      </div>

      {#if attachmentsLoading}
        <p class="muted">Carregando…</p>
      {:else if allAttachments.length === 0}
        <p class="muted">Nenhum arquivo anexado.</p>
      {:else}
        <div class="att-grid">
          {#each allAttachments as att}
            <div class="att-card">
              <button class="att-thumb att-thumb-btn" onclick={() => previewAtt = att} title="Visualizar {att.original_name}">
                {#if isImage(att.mime_type)}
                  <img src={att.url} alt={att.original_name} loading="lazy" />
                {:else}
                  <span class="att-icon">{fileIcon(att.mime_type)}</span>
                {/if}
              </button>
              <div class="att-info">
                <span class="att-name" title={att.original_name}>{att.original_name}</span>
                <a class="att-doc muted" href="#/doc/{att.document_id}" onclick={() => activeTab = ''} title={att.document_title}>{att.document_title}</a>
                <span class="att-size muted">{formatBytes(att.size_bytes)}</span>
              </div>
              <button
                class="btn btn-danger btn-sm att-del"
                onclick={() => deleteAttachment(att)}
                title="Excluir arquivo"
              >🗑</button>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Cleanup -->
  {#if activeTab === 'cleanup'}
    <div class="admin-section">
      <h3>Limpeza do banco</h3>
      <p class="muted" style="margin-bottom:.75rem">Executa VACUUM no banco de dados para recuperar espaço em disco.</p>
      <button class="btn btn-primary" onclick={handleCleanup} disabled={loading}>
        {loading ? 'Executando…' : '🧹 Iniciar VACUUM'}
      </button>
      {#if cleanupResult}
        <p class="cleanup-result">✓ VACUUM concluído.</p>
      {/if}
    </div>
  {/if}

  <!-- Links -->
  {#if activeTab === 'links'}
    <!-- URL management table -->
    <div class="admin-section">
      <div class="section-header">
        <h3>Links externos ({adminURLs ? adminURLs.length : '…'})</h3>
        <button class="btn btn-ghost btn-sm" onclick={loadAdminURLs}>↻ Atualizar</button>
      </div>

      {#if adminURLs === null}
        <p class="muted">Carregando…</p>
      {:else if adminURLs.length === 0}
        <p class="muted">Nenhum link associado a documentos.</p>
      {:else}
        <div class="url-controls">
          <input
            class="url-search"
            type="search"
            placeholder="Buscar por documento…"
            bind:value={urlSearch}
          />
          <button
            class="btn btn-ghost btn-sm url-sort-btn"
            onclick={() => urlSortDir = urlSortDir === 'asc' ? 'desc' : 'asc'}
            title="Ordenar por título do documento"
          >Documento {urlSortDir === 'asc' ? '↑' : '↓'}</button>
        </div>

        {#if filteredURLs.length === 0}
          <p class="muted" style="margin-top:.5rem">Nenhum resultado para "{urlSearch}".</p>
        {:else}
          <div class="url-table">
            <div class="url-table-head">
              <span class="utc-doc">Documento</span>
              <span class="utc-title">Título do link</span>
              <span class="utc-url">URL</span>
              <span class="utc-actions"></span>
            </div>
            {#each filteredURLs as u (u.id)}
              {#if urlEditId === u.id}
                <div class="url-row url-row-edit">
                  <span class="utc-doc">
                    <a class="doc-link" href="#{`/doc/${u.document_id}`}">{u.document_title || '(sem título)'}</a>
                  </span>
                  <input class="url-input" bind:value={urlEditForm.title} placeholder="Título" />
                  <input class="url-input" bind:value={urlEditForm.url} placeholder="https://…" />
                  <span class="utc-actions">
                    <button class="btn btn-primary btn-sm" onclick={() => saveEditURL(u)} disabled={urlSaving || !urlEditForm.url.trim()}>✓</button>
                    <button class="btn btn-ghost btn-sm" onclick={cancelEditURL}>✕</button>
                  </span>
                </div>
              {:else}
                <div class="url-row">
                  <span class="utc-doc">
                    <a class="doc-link" href="#{`/doc/${u.document_id}`}">{u.document_title || '(sem título)'}</a>
                  </span>
                  <span class="utc-title url-cell-text">{u.title || '—'}</span>
                  <a class="utc-url url-cell-text lc-url" href={u.url} target="_blank" rel="noopener">{u.url}</a>
                  <span class="utc-actions">
                    <button class="btn btn-ghost btn-sm" onclick={() => startEditURL(u)} title="Editar">✏️</button>
                    <button class="btn btn-ghost btn-sm" onclick={() => deleteAdminURL(u)} title="Excluir">🗑️</button>
                  </span>
                </div>
              {/if}
            {/each}
          </div>
        {/if}
      {/if}
    </div>

    <!-- URL validity check -->
    <div class="admin-section">
      <h3>Verificar validade dos links</h3>
      <p class="muted" style="margin-bottom:.75rem">Verifica se os links externos ainda estão acessíveis.</p>
      <button class="btn btn-primary" onclick={handleCheckLinks} disabled={linkChecking}>
        {linkChecking ? 'Verificando…' : '🔗 Verificar links'}
      </button>

      {#if linkCheckResults !== null}
        {@const invalid = linkCheckResults.filter(r => !r.valid)}
        {@const valid = linkCheckResults.filter(r => r.valid)}
        <div style="margin-top:1rem">
          <p style="margin-bottom:.5rem">
            ✅ {valid.length} válido(s) · ❌ {invalid.length} inválido(s)
          </p>
          {#if invalid.length > 0}
            <button class="btn btn-danger btn-sm" onclick={deleteInvalidLinks} style="margin-bottom:.75rem">
              Excluir todos os inválidos
            </button>
          {/if}
          <div class="link-check-list">
            {#each linkCheckResults as r}
              <div class="link-check-item {r.valid ? 'valid' : 'invalid'}">
                <span class="lc-status">{r.valid ? '✅' : '❌'}</span>
                <a href={r.url} target="_blank" rel="noopener" class="lc-url">{r.title || r.url}</a>
                {#if r.title}<span class="lc-url-small">{r.url}</span>{/if}
                <span class="lc-code">{r.status_code || 'erro'}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  {/if}

  <!-- Shares -->
  {#if activeTab === 'shares'}
    <div class="admin-section">
      <div class="section-header">
        <h3>Links públicos ativos ({shares.length})</h3>
        <button class="btn btn-ghost btn-sm" onclick={loadShares} disabled={sharesLoading}>
          {sharesLoading ? 'Carregando…' : '↻ Atualizar'}
        </button>
      </div>

      {#if sharesLoading}
        <p class="muted">Carregando…</p>
      {:else if shares.length === 0}
        <p class="muted">Nenhum link de compartilhamento ativo.</p>
      {:else}
        <div class="share-list">
          {#each shares as share}
            <div class="share-item">
              <div class="share-info">
                <a
                  class="share-doc-title"
                  href="#/doc/{share.document_id}"
                  onclick={() => activeTab = ''}
                >{share.document_title}</a>
                {#if share.url}
                  <span class="share-url muted" title={share.url}>{share.url}</span>
                {/if}
                <span class="share-date muted">
                  Criado em {new Date(share.created_at).toLocaleDateString('pt-BR', { day:'2-digit', month:'short', year:'numeric' })}
                </span>
              </div>
              <div class="share-actions">
                {#if share.url}
                  <button
                    class="btn btn-ghost btn-sm"
                    onclick={() => copyShareURL(share.url)}
                    title={share.url}
                  >{copyFeedback === share.url ? '✓ Copiado!' : '📋 Copiar'}</button>
                {/if}
                <button
                  class="btn btn-danger btn-sm"
                  onclick={() => revokeShare(share.id)}
                >Revogar</button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Storage -->
  {#if activeTab === 'storage'}
    <div class="admin-section">
      <h3>Backend de armazenamento</h3>

      {#if !storageConfig}
        <p class="muted">Carregando...</p>
      {:else}
        <div style="display:flex;align-items:center;gap:1rem;flex-wrap:wrap;margin-bottom:1.5rem">
          <span>Ativo: <strong>{storageConfig.backend === 's3' ? '☁️ S3' : '💾 Local'}</strong></span>
          {#if storageConfig.s3_configured && storageConfig.backend !== 's3'}
            <button class="btn btn-primary btn-sm" onclick={() => switchStorageBackend('s3')} disabled={storageSwitching}>
              Ativar S3
            </button>
          {/if}
          {#if storageConfig.backend === 's3'}
            <button class="btn btn-ghost btn-sm" onclick={() => switchStorageBackend('local')} disabled={storageSwitching}>
              Voltar para Local
            </button>
          {/if}
        </div>

        {#if storageConfig.s3_configured}
          <div class="storage-info">
            <p><strong>Bucket:</strong> {storageConfig.s3_bucket}</p>
            <p><strong>Região:</strong> {storageConfig.s3_region}</p>
            {#if storageConfig.s3_prefix}<p><strong>Prefixo:</strong> {storageConfig.s3_prefix}</p>{/if}
            <p><strong>Auth:</strong> {storageConfig.auth_method === 'instance_profile' ? 'IAM Role (Instance Profile)' : 'Access Key (env vars)'}</p>
          </div>
        {:else}
          <p class="muted">S3 não configurado. Defina <code>PKD_S3_BUCKET</code> e <code>PKD_S3_REGION</code> nas variáveis de ambiente.</p>
        {/if}

        <div style="display:flex;gap:.75rem;flex-wrap:wrap;margin-top:1.5rem">
          <button class="btn btn-ghost btn-sm" onclick={testStorage} disabled={storageLoading}>
            🔍 Testar conexão
          </button>
          {#if storageConfig.s3_configured}
            <button class="btn btn-ghost btn-sm" onclick={migrateStorage} disabled={storageLoading}>
              📦 Migrar para backend ativo
            </button>
            <button class="btn btn-ghost btn-sm" onclick={reconcileStorage} disabled={storageLoading} title="Verifica o bucket S3 e corrige registros desatualizados no banco">
              🔄 Reconciliar S3 ↔ DB
            </button>
            <button class="btn btn-danger btn-sm" onclick={cleanupStorageSource} disabled={storageLoading}>
              🗑 Limpar origem
            </button>
          {/if}
        </div>

        {#if storageTestResult}
          <div class="storage-result" style="margin-top:1rem">
            <h4>Resultado do teste:</h4>
            {#each Object.entries(storageTestResult) as [name, result]}
              <p>
                <strong>{name}:</strong>
                {result.ok ? '✅' : '❌'}
                {result.latency_ms}ms
                {#if result.error}<span style="color:var(--color-danger)"> — {result.error}</span>{/if}
              </p>
            {/each}
          </div>
        {/if}

        {#if storageMigrateResult}
          <div class="storage-result" style="margin-top:1rem">
            <h4>Migração:</h4>
            <p>Encontrados: <strong>{storageMigrateResult.total_found ?? '?'}</strong> · Copiados: <strong>{storageMigrateResult.copied}</strong> em {storageMigrateResult.duration_ms}ms</p>
            {#if (storageMigrateResult.total_found ?? 0) === 0}
              <p class="muted" style="font-size:.85rem">Nenhum arquivo encontrado para migrar. Se esperava arquivos no S3, use <strong>Reconciliar S3 ↔ DB</strong> primeiro para corrigir registros desatualizados.</p>
            {/if}
            {#if storageMigrateResult.errors?.length > 0}
              <p style="color:var(--color-danger)">Erros ({storageMigrateResult.errors.length}):</p>
              <ul>{#each storageMigrateResult.errors as e}<li style="font-size:.8rem;color:var(--color-danger)">{e}</li>{/each}</ul>
            {/if}
          </div>
        {/if}

        {#if storageReconcileResult}
          <div class="storage-result" style="margin-top:1rem">
            <h4>Reconciliação S3 ↔ DB:</h4>
            <p>Registros corrigidos: <strong>{storageReconcileResult.fixed}</strong> em {storageReconcileResult.duration_ms}ms</p>
            {#if storageReconcileResult.fixed > 0}
              <p class="muted" style="font-size:.85rem">✅ Registros atualizados. Execute <strong>Migrar para backend ativo</strong> agora para copiar os arquivos.</p>
            {:else}
              <p class="muted" style="font-size:.85rem">Nenhuma inconsistência encontrada entre o S3 e o banco.</p>
            {/if}
            {#if storageReconcileResult.errors?.length > 0}
              <p style="color:var(--color-danger)">Erros ({storageReconcileResult.errors.length}):</p>
              <ul>{#each storageReconcileResult.errors as e}<li style="font-size:.8rem;color:var(--color-danger)">{e}</li>{/each}</ul>
            {/if}
          </div>
        {/if}

        {#if storageCleanupResult}
          <div class="storage-result" style="margin-top:1rem">
            <h4>Limpeza da origem:</h4>
            <p>Removidos: <strong>{storageCleanupResult.removed}</strong></p>
            {#if storageCleanupResult.errors?.length > 0}
              <p style="color:var(--color-danger)">Erros:</p>
              <ul>{#each storageCleanupResult.errors as e}<li>{e}</li>{/each}</ul>
            {/if}
          </div>
        {/if}

        <hr style="margin:2rem 0">
        <h4>Backup / Restauro de anexos (local)</h4>
        <p class="muted" style="margin-bottom:1rem">Exporta ou importa todos os arquivos do backend local. Útil para migrar entre instâncias.</p>
        <div style="display:flex;gap:.75rem;flex-wrap:wrap;align-items:center">
          <button class="btn btn-ghost btn-sm" onclick={backupAttachments} disabled={storageLoading}>
            ⬇ Download ZIP de anexos
          </button>
          <input type="file" accept=".zip" bind:this={storageRestoreInput} style="font-size:.85rem">
          <button class="btn btn-ghost btn-sm" onclick={restoreAttachments} disabled={storageLoading}>
            ⬆ Restaurar ZIP
          </button>
        </div>
        {#if storageRestoreResult}
          <div class="storage-result" style="margin-top:1rem">
            <h4>Restauro:</h4>
            <p>Restaurados: <strong>{storageRestoreResult.restored}</strong></p>
            {#if storageRestoreResult.errors?.length > 0}
              <p style="color:var(--color-danger)">Erros:</p>
              <ul>{#each storageRestoreResult.errors as e}<li>{e}</li>{/each}</ul>
            {/if}
          </div>
        {/if}

        {#if storageConfig?.backend === 's3'}
          <hr style="margin:2rem 0">
          <h4>Backup assíncrono (S3)</h4>
          <p class="muted" style="margin-bottom:1rem">
            Gera um ZIP com todos os anexos diretamente no bucket S3 (sem usar disco da instância) e fornece um link de download válido por 15 minutos.
          </p>
          <div style="display:flex;gap:.75rem;flex-wrap:wrap;align-items:center">
            <button
              class="btn btn-primary btn-sm"
              onclick={startS3Backup}
              disabled={s3BackupStarting || (s3BackupJob?.state === 'running')}
            >
              {#if s3BackupStarting}
                Iniciando…
              {:else if s3BackupJob?.state === 'running'}
                Em andamento…
              {:else}
                ☁ Iniciar backup S3
              {/if}
            </button>
          </div>

          {#if s3BackupJob}
            <div class="storage-result" style="margin-top:1rem">
              <p>
                <strong>Status:</strong>
                {#if s3BackupJob.state === 'running'}
                  Em execução — {s3BackupJob.processed} / {s3BackupJob.total} anexos processados
                {:else if s3BackupJob.state === 'succeeded'}
                  Concluído — {s3BackupJob.total} anexos, {formatBytes(s3BackupJob.size_bytes)}
                {:else if s3BackupJob.state === 'failed'}
                  <span style="color:var(--color-danger)">Falhou: {s3BackupJob.error_message}</span>
                {/if}
              </p>
              {#if s3BackupJob.state === 'succeeded' && s3BackupJob.download_url}
                {#if s3BackupCountdown > 0}
                  <p>
                    <a href={s3BackupJob.download_url} target="_blank" rel="noopener" class="btn btn-primary btn-sm">
                      ⬇ Baixar ZIP
                    </a>
                    <span class="muted" style="margin-left:.75rem">Link expira em {formatCountdown(s3BackupCountdown)}</span>
                  </p>
                {:else}
                  <p>
                    <span class="muted">Link expirou.</span>
                    <button class="btn btn-ghost btn-sm" onclick={regenerateBackupURL}>Gerar nova URL</button>
                  </p>
                {/if}
              {/if}
            </div>
          {/if}
        {/if}

        <hr style="margin:2rem 0">
        <h4>Restauração assíncrona</h4>
        <p class="muted" style="margin-bottom:1rem">
          Aceita ZIP gerado por esta aplicação (mesmo formato local e S3).
          Restauração cross-backend: ZIP de produção (S3) pode ser restaurado em desenvolvimento (local).
          Apenas entradas com hash correspondente na base atual são gravadas — órfãs são reportadas.
        </p>
        <div style="display:flex;gap:.75rem;flex-wrap:wrap;align-items:center;margin-bottom:.75rem">
          <input type="file" accept=".zip" bind:this={restoreZipInput} style="font-size:.85rem">
          <label style="font-size:.9rem">
            Se chave já existir:
            <select bind:value={restoreOnConflict} style="font-size:.85rem;margin-left:.25rem">
              <option value="overwrite">Sobrescrever (padrão)</option>
              <option value="keep">Manter existente</option>
              <option value="abort">Abortar na primeira colisão</option>
            </select>
          </label>
          <button class="btn btn-primary btn-sm" onclick={startRestore} disabled={restoreStarting || (restoreJob?.state === 'running')}>
            {#if restoreStarting}
              Enviando…
            {:else if restoreJob?.state === 'running'}
              Em andamento…
            {:else}
              ⬆ Iniciar restauração
            {/if}
          </button>
        </div>

        {#if restoreJob}
          <div class="storage-result" style="margin-top:1rem">
            <p>
              <strong>Status:</strong>
              {#if restoreJob.state === 'running'}
                Em execução — {restoreJob.processed} / {restoreJob.total} entradas
              {:else if restoreJob.state === 'succeeded'}
                Concluído — {restoreJob.restore?.written ?? 0} gravados,
                {restoreJob.restore?.kept ?? 0} mantidos,
                {restoreJob.restore?.skipped_orphan ?? 0} órfãos ignorados,
                {restoreJob.restore?.hash_mismatch ?? 0} com hash inválido
              {:else if restoreJob.state === 'failed'}
                <span style="color:var(--color-danger)">Falhou: {restoreJob.error_message}</span>
              {/if}
            </p>
            {#if restoreJob.state === 'succeeded' && restoreJob.restore?.skipped?.length > 0}
              <p>
                <button class="btn btn-ghost btn-sm" onclick={() => restoreShowSkipped = !restoreShowSkipped}>
                  {restoreShowSkipped ? '▾' : '▸'} {restoreJob.restore.skipped.length} entrada(s) ignorada(s)
                </button>
              </p>
              {#if restoreShowSkipped}
                <ul style="font-family:monospace;font-size:.8rem;max-height:200px;overflow:auto">
                  {#each restoreJob.restore.skipped as e}
                    <li>{e.sha256.slice(0,16)}… ({formatBytes(e.size_bytes)}) — {e.reason}</li>
                  {/each}
                </ul>
              {/if}
            {/if}
          </div>
        {/if}
      {/if}
    </div>
  {/if}

  <!-- Preferências -->
  {#if activeTab === 'settings'}
    <div class="admin-section">
      <h3>Auto-save</h3>
      <p class="muted" style="margin-bottom:1rem">Intervalo para salvar automaticamente o documento enquanto você edita.</p>
      <div class="autosave-options">
        {#each AUTOSAVE_OPTIONS as opt}
          <label class="autosave-option {$autoSaveInterval === opt.value ? 'selected' : ''}">
            <input
              type="radio"
              name="autosave"
              value={opt.value}
              checked={$autoSaveInterval === opt.value}
              onchange={() => autoSaveInterval.set(opt.value)}
            />
            {opt.label}
          </label>
        {/each}
      </div>
    </div>

    <div class="admin-section">
      <h3>Histórico de versões</h3>
      <p class="muted" style="margin-bottom:1rem">Número máximo de versões salvas por documento. Versões mais antigas são removidas automaticamente quando o limite é atingido.</p>
      <div class="versions-setting-row">
        <input
          type="number"
          min="1"
          max="10000"
          class="versions-max-input"
          bind:value={versionsMaxPerDoc}
        />
        <span class="muted" style="font-size:.85rem">versões por documento</span>
        <button class="btn btn-primary" onclick={saveVersionsSettings} disabled={versionsSettingSaving}>
          {versionsSettingSaving ? 'Salvando…' : 'Salvar'}
        </button>
        {#if versionsSettingMsg}
          <span class="versions-setting-msg">{versionsSettingMsg}</span>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Attachment preview modal (Admin) -->
{#if previewAtt}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="preview-backdrop" onclick={() => previewAtt = null}>
    <div class="preview-box" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Visualizar arquivo">
      <div class="preview-header">
        <span class="preview-title">{previewAtt.original_name}</span>
        <div class="preview-actions">
          <a href={previewAtt.url} download={previewAtt.original_name} class="preview-dl-btn" title="Baixar">⬇ Baixar</a>
          <button class="preview-close" onclick={() => previewAtt = null} aria-label="Fechar">✕</button>
        </div>
      </div>
      <div class="preview-body">
        {#if previewType(previewAtt.mime_type) === 'image'}
          <img src={previewAtt.url} alt={previewAtt.original_name} class="preview-img" />
        {:else if previewType(previewAtt.mime_type) === 'pdf'}
          <embed src={previewAtt.url} type="application/pdf" class="preview-pdf" />
        {:else if previewType(previewAtt.mime_type) === 'audio'}
          <div class="preview-audio-wrap">
            <span class="preview-big-icon">🎵</span>
            <p class="preview-file-name">{previewAtt.original_name}</p>
            <!-- svelte-ignore a11y_media_has_caption -->
            <audio controls src={previewAtt.url} class="preview-audio"></audio>
          </div>
        {:else if previewType(previewAtt.mime_type) === 'video'}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video controls src={previewAtt.url} class="preview-video"></video>
        {:else}
          <div class="preview-download-wrap">
            <span class="preview-big-icon">{fileIcon(previewAtt.mime_type)}</span>
            <p class="preview-file-name">{previewAtt.original_name}</p>
            <p class="preview-file-size">{previewAtt.size_bytes ? (previewAtt.size_bytes / 1024).toFixed(0) + ' KB' : ''}</p>
            <a href={previewAtt.url} download={previewAtt.original_name} class="btn btn-primary">⬇ Fazer download</a>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .admin-wrap {
    flex: 1;
    overflow-y: auto;
    padding: 1.5rem 2rem;
    max-width: 800px;
    margin: 0 auto;
    width: 100%;
  }

  .admin-wrap-wide {
    max-width: none;
  }

  .autosave-options {
    display: flex;
    gap: .5rem;
    flex-wrap: wrap;
  }

  .autosave-option {
    display: flex;
    align-items: center;
    gap: .4rem;
    padding: .45rem .9rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    cursor: pointer;
    font-size: .875rem;
    transition: background .15s, border-color .15s;
  }

  .autosave-option:hover {
    background: var(--hover-bg);
  }

  .autosave-option.selected {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
    font-weight: 500;
  }

  .autosave-option input[type="radio"] {
    accent-color: var(--accent);
  }

  .versions-setting-row {
    display: flex;
    align-items: center;
    gap: .75rem;
    flex-wrap: wrap;
  }

  .versions-max-input {
    width: 80px;
    padding: .4rem .6rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    color: var(--text);
    font-size: .875rem;
  }

  .versions-setting-msg {
    font-size: .85rem;
    color: var(--accent);
  }

  .admin-title { font-size: 1.25rem; font-weight: 700; margin-bottom: 1.25rem; }

  .tabs {
    display: flex;
    flex-wrap: wrap;
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
    gap: .5rem;
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

  /* Tag table */
  .tag-table {
    display: flex;
    flex-direction: column;
    gap: .3rem;
  }

  .tag-row {
    display: flex;
    align-items: center;
    gap: .5rem;
    padding: .3rem .4rem;
    border-radius: var(--radius);
    background: var(--bg-hover);
    font-size: .875rem;
  }

  .tag-color-dot {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    flex-shrink: 0;
    border: 1px solid rgba(0,0,0,.15);
  }

  .color-input {
    width: 32px;
    height: 28px;
    padding: 1px 2px;
    border-radius: 4px;
    border: 1px solid var(--border);
    cursor: pointer;
    flex-shrink: 0;
    background: none;
  }

  .tag-name { flex: 1; font-weight: 500; }
  .tag-count { font-size: .8rem; flex-shrink: 0; }

  .tag-name-input {
    flex: 1;
    padding: .3rem .6rem;
    border-radius: 4px;
    font-size: .875rem;
    min-width: 80px;
  }

  .tag-msg { font-size: .875rem; color: var(--success); }

  .rename-row {
    display: flex;
    align-items: center;
    gap: .5rem;
    flex-wrap: wrap;
  }
  .rename-row input { flex: 1; min-width: 120px; padding: .45rem .75rem; }
  .rename-msg { margin-top: .5rem; font-size: .875rem; color: var(--success); }

  /* Attachment grid */
  .att-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: .75rem;
    margin-top: .5rem;
  }

  .att-card {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background: var(--bg-hover);
    position: relative;
  }

  .att-thumb {
    height: 100px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    overflow: hidden;
  }

  .att-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .att-icon { font-size: 2.5rem; }

  .att-info {
    padding: .4rem .5rem;
    display: flex;
    flex-direction: column;
    gap: .1rem;
    flex: 1;
    min-width: 0;
  }

  .att-name {
    font-size: .8rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .att-doc {
    font-size: .75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: inherit;
    text-decoration: none;
  }
  a.att-doc:hover { text-decoration: underline; }

  .att-size { font-size: .7rem; }

  .att-del {
    position: absolute;
    top: .25rem;
    right: .25rem;
    opacity: 0;
    transition: opacity .15s;
    padding: .2rem .4rem;
    font-size: .75rem;
  }
  .att-card:hover .att-del { opacity: 1; }

  .orphan-section {
    border: 1px solid var(--warning, #f59e0b);
    border-radius: var(--radius);
    padding: .75rem 1rem;
    background: rgba(245, 158, 11, .06);
  }

  .orphan-list {
    display: flex;
    flex-direction: column;
    gap: .4rem;
    margin-top: .5rem;
  }

  .orphan-row {
    display: flex;
    align-items: center;
    gap: .75rem;
    padding: .35rem .5rem;
    border-radius: var(--radius-sm, 4px);
    background: var(--surface-alt, rgba(0,0,0,.04));
  }

  .orphan-info {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: .1rem;
    min-width: 0;
  }

  .orphan-name {
    font-size: .875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .orphan-name-btn {
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    color: var(--accent);
    text-align: left;
    text-decoration: underline dotted;
  }
  .orphan-name-btn:hover { text-decoration: underline; }

  .orphan-doc {
    font-size: .75rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .orphan-reason {
    font-size: .75rem;
    white-space: nowrap;
    padding: .1rem .35rem;
    border-radius: 3px;
    background: var(--surface-alt, rgba(0,0,0,.06));
  }

  .orphan-size {
    font-size: .8rem;
    white-space: nowrap;
  }

  .cleanup-result {
    margin-top: .75rem;
    color: var(--success);
    font-size: .9rem;
  }

  .btn-sm { padding: .25rem .6rem; font-size: .8rem; }
  .muted { color: var(--text-muted); }

  /* URL management table */
  .url-controls {
    display: flex;
    align-items: center;
    gap: .5rem;
    margin-bottom: .75rem;
  }

  .url-search {
    flex: 1;
    padding: .4rem .75rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    font-size: .875rem;
  }

  .url-sort-btn { flex-shrink: 0; }

  .url-table {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    font-size: .875rem;
  }

  .url-table-head {
    display: grid;
    grid-template-columns: minmax(180px, 2fr) minmax(120px, 1fr) minmax(200px, 3fr) 88px;
    gap: .75rem;
    padding: .45rem 1rem;
    background: var(--bg-hover);
    font-weight: 600;
    font-size: .8rem;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border);
  }

  .url-row {
    display: grid;
    grid-template-columns: minmax(180px, 2fr) minmax(120px, 1fr) minmax(200px, 3fr) 88px;
    gap: .75rem;
    align-items: center;
    padding: .45rem 1rem;
    border-bottom: 1px solid var(--border);
  }

  .url-row:last-child { border-bottom: none; }
  .url-row:hover:not(.url-row-edit) { background: var(--bg-hover); }
  .url-row-edit { background: var(--bg-hover); }

  .utc-doc { overflow: hidden; }
  .utc-title { overflow: hidden; }
  .utc-url { overflow: hidden; }
  .utc-actions { display: flex; gap: .25rem; justify-content: flex-end; flex-shrink: 0; }

  .url-cell-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }

  .url-input {
    width: 100%;
    padding: .3rem .5rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text);
    font-size: .875rem;
    box-sizing: border-box;
  }

  .doc-link {
    color: var(--accent);
    text-decoration: none;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
  }
  .doc-link:hover { text-decoration: underline; }

  .link-check-list {
    display: flex;
    flex-direction: column;
    gap: .375rem;
    max-height: 400px;
    overflow-y: auto;
  }

  .link-check-item {
    display: flex;
    align-items: center;
    gap: .5rem;
    padding: .35rem .5rem;
    border-radius: 4px;
    font-size: .875rem;
    background: var(--bg-hover);
  }

  .link-check-item.invalid { background: rgba(239, 68, 68, .1); }
  .lc-status { flex-shrink: 0; }
  .lc-url { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--accent); }
  .lc-url-small { font-size: .75rem; color: var(--text-muted); max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .lc-code { flex-shrink: 0; font-size: .75rem; color: var(--text-muted); font-family: monospace; }

  /* Clickable attachment thumb */
  .att-thumb-btn {
    width: 100%;
    padding: 0;
    border: none;
    background: none;
    cursor: pointer;
  }
  .att-thumb-btn:hover { opacity: .85; }

  /* Shares tab */
  .share-list {
    display: flex;
    flex-direction: column;
    gap: .4rem;
  }

  .share-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: .75rem;
    padding: .5rem .75rem;
    border-radius: var(--radius);
    background: var(--bg-hover);
    font-size: .875rem;
  }

  .share-info {
    display: flex;
    flex-direction: column;
    gap: .1rem;
    min-width: 0;
  }

  .share-doc-title {
    font-weight: 500;
    color: var(--accent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .share-url {
    font-size: .72rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: monospace;
  }

  .share-date { font-size: .8rem; }

  .share-actions {
    display: flex;
    align-items: center;
    gap: .375rem;
    flex-shrink: 0;
  }

  /* Preview modal (reused from Editor) */
  .preview-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,.65);
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1rem;
  }

  .preview-box {
    background: var(--bg-panel);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    max-width: 90vw;
    max-height: 90vh;
    width: 800px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .preview-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: .75rem 1rem;
    border-bottom: 1px solid var(--border);
    gap: .5rem;
    flex-shrink: 0;
  }

  .preview-title {
    font-weight: 500;
    font-size: .9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .preview-actions { display: flex; align-items: center; gap: .5rem; flex-shrink: 0; }

  .preview-dl-btn {
    font-size: .8rem;
    color: var(--accent);
    padding: .25rem .5rem;
    border-radius: var(--radius);
  }
  .preview-dl-btn:hover { background: var(--bg-hover); }

  .preview-close {
    width: 28px; height: 28px;
    border-radius: 50%;
    font-size: .9rem;
    color: var(--text-muted);
    display: flex; align-items: center; justify-content: center;
  }
  .preview-close:hover { background: var(--bg-hover); color: var(--text); }

  .preview-body {
    flex: 1;
    overflow: auto;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
    min-height: 200px;
  }

  .preview-img { max-width: 100%; max-height: 75vh; object-fit: contain; }
  .preview-pdf { width: 100%; height: 75vh; border: none; }
  .preview-video { max-width: 100%; max-height: 75vh; }

  .preview-audio-wrap, .preview-download-wrap {
    display: flex; flex-direction: column; align-items: center;
    gap: 1rem; padding: 2rem;
  }

  .preview-big-icon { font-size: 4rem; }
  .preview-file-name { font-size: .9rem; font-weight: 500; text-align: center; }
  .preview-file-size { font-size: .8rem; color: var(--text-muted); }
  .preview-audio { width: 300px; max-width: 100%; }
</style>
