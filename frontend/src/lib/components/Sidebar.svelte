<script>
  import { onMount } from 'svelte'
  import TreeNode from './TreeNode.svelte'
  import { tree, loadTree, createDoc, sortTree, treeExpansionSignal, tagFilter, favoriteFilter, textFilter, viewMode } from '../stores/documents.js'
  import { tags, loadTags } from '../stores/tags.js'

  let { onNavigate, onClearFilter } = $props()

  let selectedTags = $state([])
  let currentHash = $state(window.location.hash)

  // Keep selectedTags in sync when tagFilter is reset externally (e.g. topbar reset button)
  $effect(() => { selectedTags = [...$tagFilter] })

  onMount(() => {
    loadTree()
    loadTags()
    window.addEventListener('hashchange', () => { currentHash = window.location.hash })
  })

  function getActiveId() {
    const m = currentHash.match(/^#\/doc\/(\d+)/)
    return m ? Number(m[1]) : null
  }

  function toggleTag(name) {
    if (selectedTags.includes(name)) {
      selectedTags = selectedTags.filter(t => t !== name)
    } else {
      selectedTags = [...selectedTags, name]
    }
    loadTree(selectedTags, $favoriteFilter)
  }

  function setViewMode(mode) {
    viewMode.set(mode)
    loadTree()
  }

  async function handleNewRoot() {
    const doc = await createDoc(null)
    navigate(doc.id)
  }

  function navigate(id) {
    window.location.hash = `/doc/${id}`
    onNavigate?.(id)
  }

  function expandAll() {
    treeExpansionSignal.set('expand')
    setTimeout(() => treeExpansionSignal.set(null), 0)
  }

  function collapseAll() {
    treeExpansionSignal.set('collapse')
    setTimeout(() => treeExpansionSignal.set(null), 0)
  }
</script>

<div class="sidebar-inner">
  <!-- View mode toggle (hidden during search) -->
  {#if !$textFilter}
    <div class="view-toggle" role="group" aria-label="Modo de visualização">
      <button class="view-btn {$viewMode === 'active' || !$viewMode ? 'active' : ''}" onclick={() => setViewMode('active')}>Ativos</button>
      <button class="view-btn {$viewMode === 'archived' ? 'active' : ''}" onclick={() => setViewMode('archived')}>Arquivados</button>
      <button class="view-btn {$viewMode === 'all' ? 'active' : ''}" onclick={() => setViewMode('all')}>Todos</button>
    </div>
  {/if}

  <!-- Expand/collapse toolbar (hidden while text filter is active) -->
  {#if !$textFilter}
    <div class="sidebar-toolbar">
      <button class="expand-btn" onclick={expandAll} title="Expandir tudo" aria-label="Expandir tudo">▾</button>
      <button class="expand-btn" onclick={collapseAll} title="Recolher tudo" aria-label="Recolher tudo">▸</button>
      <button class="expand-btn" onclick={() => sortTree('alpha')} title="Ordenar A-Z">A-Z</button>
      <button class="expand-btn" onclick={() => sortTree('created')} title="Ordenar por data de criação">📅</button>
      <button class="expand-btn {$favoriteFilter ? 'fav-active' : ''}" onclick={() => loadTree(selectedTags, !$favoriteFilter)} title={$favoriteFilter ? 'Mostrar todos' : 'Somente favoritos'} aria-label="Filtrar favoritos">⭐</button>
    </div>
  {/if}

  <!-- Tag filters (hidden while text filter is active) -->
  {#if !$textFilter && $tags.length > 0}
    <div class="tag-filter" aria-label="Filtrar por tag">
      {#each $tags as tag}
        {@const isActive = selectedTags.includes(tag.name)}
        {@const c = tag.color || ''}
        <button
          class="tag-chip {isActive ? 'active' : ''}"
          style={c && isActive
            ? `background:${c}; border-color:${c}; color:#fff`
            : c
            ? `background:${c}22; border-color:${c}; color:${c}`
            : ''}
          onclick={() => toggleTag(tag.name)}
          title="#{tag.name} — {tag.count} documentos"
        >
          #{tag.name}
        </button>
      {/each}
    </div>
  {/if}

  <!-- Filter active banner -->
  {#if $textFilter}
    <div class="filter-banner">
      <span class="filter-label">"{$textFilter}"</span>
      <button class="filter-clear-link" onclick={onClearFilter}>Todos os documentos</button>
    </div>
  {/if}

  <!-- Document tree (always shown; filtered when query is active) -->
  <nav aria-label={$textFilter ? 'Resultados do filtro' : 'Árvore de documentos'} class="tree-nav">
    {#each $tree as node (node.id)}
      <TreeNode {node} activeId={getActiveId()} {navigate} onNavigate={navigate} />
    {/each}
    {#if $tree.length === 0}
      {#if $textFilter}
        <p class="tree-empty">Sem resultados para "{$textFilter}"</p>
      {:else if $viewMode === 'archived'}
        <p class="tree-empty">Nenhum documento arquivado.</p>
      {:else}
        <p class="tree-empty">Nenhum documento ainda.</p>
      {/if}
    {/if}
  </nav>

  <!-- New root document -->
  <div class="new-root">
    <button class="new-doc-btn" onclick={handleNewRoot}>
      + Novo documento
    </button>
  </div>
</div>

<style>
  .view-toggle {
    display: flex;
    padding: .3rem .5rem;
    gap: 2px;
    border-bottom: 1px solid var(--border);
  }

  .view-btn {
    flex: 1;
    padding: .25rem .4rem;
    font-size: .73rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition: background .12s, color .12s;
  }

  .view-btn:hover { background: var(--bg-hover); color: var(--text); }
  .view-btn.active { background: var(--accent); border-color: var(--accent); color: #fff; }

  .sidebar-toolbar {
    display: flex;
    align-items: center;
    gap: .25rem;
    padding: .375rem .5rem;
    border-bottom: 1px solid var(--border);
  }

  .expand-btn {
    flex-shrink: 0;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: .75rem;
    padding: .2rem .3rem;
    border-radius: var(--radius);
  }

  .expand-btn:hover { background: var(--bg-hover); color: var(--text); }
  .expand-btn.fav-active { color: #f5c518; }

  .tag-filter {
    display: flex;
    flex-wrap: wrap;
    gap: .375rem;
    padding: .5rem .75rem .5rem;
    border-bottom: 1px solid var(--border);
  }

  .filter-banner {
    display: flex;
    align-items: center;
    gap: .5rem;
    padding: .3rem .75rem;
    background: var(--bg-hover);
    border-bottom: 1px solid var(--border);
    font-size: .75rem;
  }

  .filter-label {
    flex: 1;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-style: italic;
  }

  .filter-clear-link {
    flex-shrink: 0;
    background: none;
    border: none;
    color: var(--accent);
    cursor: pointer;
    font-size: .75rem;
    padding: 0;
    text-decoration: underline;
  }

  .filter-clear-link:hover { opacity: .8; }

  .tree-nav { flex: 1; overflow-y: auto; }

  .tree-empty {
    padding: 1rem .75rem;
    color: var(--text-muted);
    font-size: .875rem;
  }

  .new-root {
    border-top: 1px solid var(--border);
    padding: .375rem .5rem;
  }

  .new-doc-btn {
    width: 100%;
    text-align: left;
    padding: .4rem .5rem;
    border-radius: var(--radius);
    font-size: .875rem;
    color: var(--text-muted);
    cursor: pointer;
  }

  .new-doc-btn:hover { background: var(--bg-hover); color: var(--text); }
</style>
