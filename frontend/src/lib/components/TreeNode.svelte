<script>
  import { createDoc, trashDoc, moveDoc, reorderDoc, findNextSiblingId, treeExpansionSignal, linksRefreshSignal, toggleFavorite } from '../stores/documents.js'
  import { apiPost } from '../api.js'

  let {
    node,
    activeId,
    depth = 0,
    onNavigate,
  } = $props()

  let expanded = $state(true)
  let dropZone = $state(null) // 'before' | 'inside' | 'after'
  let linkAdding = $state(false)

  $effect(() => {
    if ($treeExpansionSignal === 'expand') expanded = true
    else if ($treeExpansionSignal === 'collapse') expanded = false
  })

  function navigate() {
    onNavigate(node.id)
  }

  async function handleNewChild(e) {
    e.stopPropagation()
    const doc = await createDoc(node.id)
    onNavigate(doc.id)
  }

  async function handleDelete(e) {
    e.stopPropagation()
    if (confirm(`Mover "${node.title}" para a lixeira?`)) {
      await trashDoc(node.id)
    }
  }

  async function handleToggleFavorite(e) {
    e.stopPropagation()
    await toggleFavorite(node.id)
  }

  async function handleRelate(e) {
    e.stopPropagation()
    if (!activeId || activeId === node.id || linkAdding) return
    linkAdding = true
    try {
      await apiPost(`/api/documents/${activeId}/links`, { target_id: node.id })
      linksRefreshSignal.update(n => n + 1)
    } catch (err) {
      if (!err.message?.includes('already exists')) {
        console.error('Erro ao relacionar:', err)
      }
    } finally {
      linkAdding = false
    }
  }

  function onDragStart(e) {
    e.dataTransfer.setData('text/plain', String(node.id))
    e.dataTransfer.effectAllowed = 'move'
  }

  function onDragOver(e) {
    e.preventDefault()
    const rect = e.currentTarget.getBoundingClientRect()
    const relY = (e.clientY - rect.top) / rect.height
    dropZone = relY < 0.3 ? 'before' : relY > 0.7 ? 'after' : 'inside'
  }

  function onDragLeave(e) {
    if (!e.currentTarget.contains(e.relatedTarget)) {
      dropZone = null
    }
  }

  async function onDrop(e) {
    e.preventDefault()
    const zone = dropZone
    dropZone = null
    const draggedId = Number(e.dataTransfer.getData('text/plain'))
    if (!draggedId || draggedId === node.id) return

    if (zone === 'inside') {
      await moveDoc(draggedId, node.id)
    } else if (zone === 'before') {
      await reorderDoc(draggedId, node.parent_id, node.id)
    } else {
      const nextId = findNextSiblingId(node.id, node.parent_id)
      await reorderDoc(draggedId, node.parent_id, nextId)
    }
  }
</script>

<div>
  <!-- Row -->
  <div
    class="tree-item {node.id === activeId ? 'active' : ''} {dropZone === 'inside' ? 'drag-over' : ''} {dropZone === 'before' ? 'drop-before' : ''} {dropZone === 'after' ? 'drop-after' : ''}"
    style="padding-left: {0.4 + depth * 0.75}rem"
    onclick={navigate}
    draggable="true"
    ondragstart={onDragStart}
    ondragover={onDragOver}
    ondragleave={onDragLeave}
    ondrop={onDrop}
    role="button"
    tabindex="0"
    onkeydown={e => e.key === 'Enter' && navigate()}
  >
    <!-- Toggle -->
    <button
      class="toggle-btn"
      onclick={e => { e.stopPropagation(); expanded = !expanded }}
      aria-label={expanded ? 'Recolher' : 'Expandir'}
    >
      {#if node.children?.length}
        {expanded ? '▾' : '▸'}
      {:else}
        &nbsp;
      {/if}
    </button>

    <i class="bx {node.icon || 'bx-file-blank'} icon"></i>
    <span class="label">{node.title || 'Sem título'}</span>

    <button
      class="star-btn {node.is_favorite ? 'is-favorite' : ''}"
      onclick={handleToggleFavorite}
      title={node.is_favorite ? 'Remover dos favoritos' : 'Marcar como favorito'}
      aria-label={node.is_favorite ? 'Remover dos favoritos' : 'Marcar como favorito'}
    >{node.is_favorite ? '⭐' : '☆'}</button>

    <!-- Actions (shown on hover) -->
    <span class="row-actions">
      {#if activeId && activeId !== node.id}
        <button
          class="row-btn row-btn-relate {linkAdding ? 'adding' : ''}"
          onclick={handleRelate}
          title="Relacionar com documento atual"
          aria-label="Relacionar"
        >→</button>
      {/if}
      <button class="row-btn" onclick={handleNewChild} title="Novo filho">+</button>
      <button class="row-btn row-btn-del" onclick={handleDelete} title="Lixeira">×</button>
    </span>
  </div>

  <!-- Children -->
  {#if expanded && node.children?.length}
    <div>
      {#each node.children as child}
        <svelte:self
          node={child}
          {activeId}
          depth={depth + 1}
          {onNavigate}
        />
      {/each}
    </div>
  {/if}
</div>

<style>
  .toggle-btn {
    width: 14px;
    font-size: .65rem;
    color: var(--text-muted);
    flex-shrink: 0;
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
    line-height: 1;
  }

  .row-actions {
    display: none;
    margin-left: auto;
    gap: 1px;
  }

  .tree-item:hover .row-actions { display: flex; }

  .row-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 3px;
    font-size: .8rem;
    color: var(--text-muted);
    cursor: pointer;
  }
  .row-btn:hover { background: var(--bg-active); color: var(--text); }
  .row-btn-del:hover { color: var(--danger); }
  .row-btn-relate:hover { color: var(--accent); }
  .row-btn-relate.adding { opacity: .5; cursor: default; }

  .star-btn {
    flex-shrink: 0;
    background: none;
    border: none;
    cursor: pointer;
    font-size: .75rem;
    opacity: 0;
    padding: 0 2px;
    line-height: 1;
    color: var(--text-muted);
  }
  .tree-item:hover .star-btn { opacity: .6; }
  .star-btn.is-favorite { opacity: 1; color: #f5c518; }
  .star-btn:hover { opacity: 1 !important; }

  .drag-over  { background: var(--bg-active); outline: 2px dashed var(--accent); }
  .drop-before { box-shadow: 0 -2px 0 0 var(--accent); }
  .drop-after  { box-shadow: 0  2px 0 0 var(--accent); }
</style>
