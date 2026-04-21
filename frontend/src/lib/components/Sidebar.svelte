<script>
  import { onMount } from 'svelte'
  import TreeNode from './TreeNode.svelte'
  import { tree, loadTree, createDoc, sortTree, treeExpansionSignal, favoriteFilter } from '../stores/documents.js'
  import { tags, loadTags } from '../stores/tags.js'

  let { onNavigate } = $props()

  let selectedTags = $state([])
  let currentHash = $state(window.location.hash)
  let sidebarQuery = $state('')
  let filterTimer = null

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
    loadTree(selectedTags, $favoriteFilter, sidebarQuery)
  }

  async function handleNewRoot() {
    const doc = await createDoc(null)
    navigate(doc.id)
  }

  function navigate(id) {
    window.location.hash = `/doc/${id}`
    onNavigate?.(id)
  }

  function onFilterInput(e) {
    sidebarQuery = e.target.value
    clearTimeout(filterTimer)
    filterTimer = setTimeout(() => {
      loadTree(selectedTags, $favoriteFilter, sidebarQuery)
    }, 200)
  }

  function clearFilter() {
    sidebarQuery = ''
    clearTimeout(filterTimer)
    loadTree(selectedTags, $favoriteFilter, '')
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
  <!-- Filter + expand/collapse toolbar -->
  <div class="sidebar-toolbar">
    <div class="search-wrap">
      <input
        class="sidebar-search"
        type="search"
        placeholder="Filtrar documentos…"
        autocomplete="off"
        value={sidebarQuery}
        oninput={onFilterInput}
        aria-label="Filtrar documentos"
      />
      {#if sidebarQuery}
        <button class="clear-btn" onclick={clearFilter} aria-label="Limpar">×</button>
      {/if}
    </div>
    {#if !sidebarQuery}
      <button class="expand-btn" onclick={expandAll} title="Expandir tudo" aria-label="Expandir tudo">▾</button>
      <button class="expand-btn" onclick={collapseAll} title="Recolher tudo" aria-label="Recolher tudo">▸</button>
      <button class="expand-btn" onclick={() => sortTree('alpha')} title="Ordenar A-Z">A-Z</button>
      <button class="expand-btn" onclick={() => sortTree('created')} title="Ordenar por data de criação">📅</button>
      <button class="expand-btn {$favoriteFilter ? 'fav-active' : ''}" onclick={() => loadTree(selectedTags, !$favoriteFilter, sidebarQuery)} title={$favoriteFilter ? 'Mostrar todos' : 'Somente favoritos'} aria-label="Filtrar favoritos">⭐</button>
    {/if}
  </div>

  <!-- Tag filters (hidden while text filter is active) -->
  {#if !sidebarQuery && $tags.length > 0}
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
  {#if sidebarQuery}
    <div class="filter-banner">
      <span class="filter-label">"{sidebarQuery}"</span>
      <button class="filter-clear-link" onclick={clearFilter}>Todos os documentos</button>
    </div>
  {/if}

  <!-- Document tree (always shown; filtered when query is active) -->
  <nav aria-label={sidebarQuery ? 'Resultados do filtro' : 'Árvore de documentos'} class="tree-nav">
    {#each $tree as node}
      <TreeNode {node} activeId={getActiveId()} {navigate} onNavigate={navigate} />
    {/each}
    {#if $tree.length === 0}
      {#if sidebarQuery}
        <p class="tree-empty">Sem resultados para "{sidebarQuery}"</p>
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
  .sidebar-toolbar {
    display: flex;
    align-items: center;
    gap: .25rem;
    padding: .375rem .5rem;
    border-bottom: 1px solid var(--border);
  }

  .search-wrap {
    position: relative;
    flex: 1;
    min-width: 0;
  }

  .sidebar-search {
    width: 100%;
    padding: .3rem .5rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: .8rem;
    background: var(--bg);
    color: var(--text);
  }

  .sidebar-search:focus { outline: none; border-color: var(--accent); }

  .clear-btn {
    position: absolute;
    right: .25rem;
    top: 50%;
    transform: translateY(-50%);
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: .9rem;
    line-height: 1;
    padding: .1rem .2rem;
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
