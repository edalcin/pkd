<script>
  import { onMount } from 'svelte'
  import TreeNode from './TreeNode.svelte'
  import { tree, loadTree, createDoc, treeExpansionSignal } from '../stores/documents.js'
  import { tags, loadTags } from '../stores/tags.js'
  import { apiGet } from '../api.js'

  let { onNavigate } = $props()

  let selectedTags = $state([])
  let currentHash = $state(window.location.hash)
  let sidebarQuery = $state('')
  let sidebarResults = $state([])
  let sidebarSearching = $state(false)
  let searchTimer = null

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
    loadTree(selectedTags)
  }

  async function handleNewRoot() {
    const doc = await createDoc(null)
    onNavigate?.(doc.id)
  }

  function navigate(id) {
    window.location.hash = `/doc/${id}`
    onNavigate?.(id)
  }

  function onSearchInput(e) {
    sidebarQuery = e.target.value
    clearTimeout(searchTimer)
    if (!sidebarQuery.trim()) {
      sidebarResults = []
      return
    }
    searchTimer = setTimeout(async () => {
      sidebarSearching = true
      try {
        sidebarResults = await apiGet(`/api/search?q=${encodeURIComponent(sidebarQuery)}`)
      } catch {
        sidebarResults = []
      } finally {
        sidebarSearching = false
      }
    }, 200)
  }

  function clearSidebarSearch() {
    sidebarQuery = ''
    sidebarResults = []
    clearTimeout(searchTimer)
  }

  function selectResult(id) {
    navigate(id)
    clearSidebarSearch()
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
  <!-- Search + expand/collapse toolbar -->
  <div class="sidebar-toolbar">
    <div class="search-wrap">
      <input
        class="sidebar-search"
        type="search"
        placeholder="Filtrar documentos…"
        autocomplete="off"
        value={sidebarQuery}
        oninput={onSearchInput}
        aria-label="Filtrar documentos"
      />
      {#if sidebarQuery}
        <button class="clear-btn" onclick={clearSidebarSearch} aria-label="Limpar">×</button>
      {/if}
    </div>
    {#if !sidebarQuery}
      <button class="expand-btn" onclick={expandAll} title="Expandir tudo" aria-label="Expandir tudo">▾</button>
      <button class="expand-btn" onclick={collapseAll} title="Recolher tudo" aria-label="Recolher tudo">▸</button>
    {/if}
  </div>

  <!-- Tag filters (hidden while searching) -->
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

  <!-- Document tree or search results -->
  {#if sidebarQuery}
    <nav aria-label="Resultados da busca" class="tree-nav">
      {#if sidebarSearching}
        <div class="tree-empty"><div class="spinner" style="width:16px;height:16px;margin:auto"></div></div>
      {:else if sidebarResults.length === 0}
        <p class="tree-empty">Sem resultados para "{sidebarQuery}"</p>
      {:else}
        {#each sidebarResults as result}
          <div
            class="tree-item search-hit {result.id === getActiveId() ? 'active' : ''}"
            onclick={() => selectResult(result.id)}
            role="button"
            tabindex="0"
            onkeydown={e => e.key === 'Enter' && selectResult(result.id)}
          >
            <span class="icon">📄</span>
            <span class="label">{result.title || 'Sem título'}</span>
          </div>
        {/each}
      {/if}
    </nav>
  {:else}
    <nav aria-label="Árvore de documentos" class="tree-nav">
      {#each $tree as node}
        <TreeNode {node} activeId={getActiveId()} {navigate} onNavigate={navigate} />
      {/each}
      {#if $tree.length === 0}
        <p class="tree-empty">Nenhum documento ainda.</p>
      {/if}
    </nav>
  {/if}

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

  .tag-filter {
    display: flex;
    flex-wrap: wrap;
    gap: .375rem;
    padding: .5rem .75rem .5rem;
    border-bottom: 1px solid var(--border);
  }

  .tree-nav { flex: 1; overflow-y: auto; }

  .tree-empty {
    padding: 1rem .75rem;
    color: var(--text-muted);
    font-size: .875rem;
  }

  .search-hit {
    padding: .4rem .75rem;
    display: flex;
    align-items: center;
    gap: .4rem;
    cursor: pointer;
    font-size: .875rem;
    border-radius: var(--radius);
    margin: .1rem .25rem;
  }

  .search-hit .icon { flex-shrink: 0; }
  .search-hit .label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .search-hit:hover { background: var(--bg-hover); }
  .search-hit.active { background: var(--bg-active); color: var(--accent); font-weight: 500; }

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
