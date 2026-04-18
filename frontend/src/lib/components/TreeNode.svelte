<script>
  import { createDoc, trashDoc, moveDoc, loadTree, treeExpansionSignal, linksRefreshSignal } from '../stores/documents.js'
  import { apiPost } from '../api.js'

  let {
    node,
    activeId,
    depth = 0,
    onNavigate,
  } = $props()

  let expanded = $state(true)
  let draggingOver = $state(false)
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
    draggingOver = true
  }

  function onDragLeave() {
    draggingOver = false
  }

  async function onDrop(e) {
    e.preventDefault()
    draggingOver = false
    const draggedId = Number(e.dataTransfer.getData('text/plain'))
    if (draggedId && draggedId !== node.id) {
      await moveDoc(draggedId, node.id)
    }
  }
</script>

<div>
  <!-- Row -->
  <div
    class="tree-item {node.id === activeId ? 'active' : ''} {draggingOver ? 'drag-over' : ''}"
    style="padding-left: {0.75 + depth * 1}rem"
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

    <span class="icon">{node.icon || '📄'}</span>
    <span class="label">{node.title || 'Sem título'}</span>

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
    width: 16px;
    font-size: .7rem;
    color: var(--text-muted);
    flex-shrink: 0;
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
  }

  .row-actions {
    display: none;
    margin-left: auto;
    gap: 2px;
  }

  .tree-item:hover .row-actions { display: flex; }

  .row-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 3px;
    font-size: .85rem;
    color: var(--text-muted);
    cursor: pointer;
  }
  .row-btn:hover { background: var(--bg-active); color: var(--text); }
  .row-btn-del:hover { color: var(--danger); }
  .row-btn-relate:hover { color: var(--accent); }
  .row-btn-relate.adding { opacity: .5; cursor: default; }

  .drag-over { background: var(--bg-active); outline: 2px dashed var(--accent); }
</style>
