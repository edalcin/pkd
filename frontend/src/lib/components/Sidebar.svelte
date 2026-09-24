<script>
  import { onMount } from 'svelte'
  import TreeNode from './TreeNode.svelte'
  import { tree, loadTree, createDoc, sortTree, treeExpansionSignal, tagFilter, favoriteFilter, textFilter, viewMode } from '../stores/documents.js'
  import { tags, loadTags } from '../stores/tags.js'
  import { memories, loadMemories, groupMemories, memoryTimeLabel } from '../stores/memories.js'
  import NewMemoryDialog from './NewMemoryDialog.svelte'

  let { onNavigate, onClearFilter } = $props()

  let selectedTags = $state([])
  let currentHash = $state(window.location.hash)
  let tagsCollapsed = $state(localStorage.getItem('pkd-tags-collapsed') === 'true')
  let mcCollapsed = $state(localStorage.getItem('pkd-mc-collapsed') === 'true')
  let expandedYears = $state(new Set())   // opt-in: years start collapsed
  let collapsedMonths = $state(new Set()) // opt-in: months/days start expanded
  let collapsedDays = $state(new Set())
  let newMemoryOpen = $state(false)

  const mcYears = $derived(groupMemories($memories))

  // Keep selectedTags in sync when tagFilter is reset externally (e.g. topbar reset button)
  $effect(() => { selectedTags = [...$tagFilter] })

  onMount(() => {
    loadTree()
    loadTags()
    loadMemories()
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

  function toggleTagsCollapse() {
    tagsCollapsed = !tagsCollapsed
    localStorage.setItem('pkd-tags-collapsed', String(tagsCollapsed))
  }

  function toggleMcCollapse() {
    mcCollapsed = !mcCollapsed
    localStorage.setItem('pkd-mc-collapsed', String(mcCollapsed))
  }

  function toggleYear(y) {
    const s = new Set(expandedYears)
    s.has(y) ? s.delete(y) : s.add(y)
    expandedYears = s
  }

  function toggleMonth(key) {
    const s = new Set(collapsedMonths)
    s.has(key) ? s.delete(key) : s.add(key)
    collapsedMonths = s
  }

  function toggleDay(key) {
    const s = new Set(collapsedDays)
    s.has(key) ? s.delete(key) : s.add(key)
    collapsedDays = s
  }

  function handleMemoryCreated(doc) {
    newMemoryOpen = false
    navigate(doc.id)
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
    <div class="tag-section">
      <button class="tag-section-header" onclick={toggleTagsCollapse} aria-expanded={!tagsCollapsed} aria-controls="tag-filter-list">
        <span class="tag-section-label">Tags</span>
        <span class="tag-section-arrow">{tagsCollapsed ? '▸' : '▾'}</span>
      </button>
      {#if !tagsCollapsed}
        <div class="tag-filter" id="tag-filter-list" aria-label="Filtrar por tag">
          {#each $tags as tag}
            {@const isActive = selectedTags.includes(tag.name)}
            {@const c = tag.color || ''}
            {@const tc = tag.text_color || ''}
            <button
              class="tag-chip {isActive ? 'active' : ''}"
              style={c && isActive
                ? `background:${c}; border-color:${c}; color:${tc || '#fff'}`
                : c
                ? `background:${c}22; border-color:${c}; color:${tc || c}`
                : ''}
              onclick={() => toggleTag(tag.name)}
              title="#{tag.name} — {tag.count} documentos"
            >
              #{tag.name}
            </button>
          {/each}
        </div>
      {/if}
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

  <!-- Memória Cronológica (hidden while text filter is active: search already
       mixes Memórias into the normal tree above) -->
  {#if !$textFilter}
    <div class="mc-section">
      <button class="tag-section-header" onclick={toggleMcCollapse} aria-expanded={!mcCollapsed} aria-controls="mc-tree">
        <span class="tag-section-label">Memória Cronológica</span>
        <span class="tag-section-arrow">{mcCollapsed ? '▸' : '▾'}</span>
      </button>
      {#if !mcCollapsed}
        <nav class="mc-tree" id="mc-tree" aria-label="Memória Cronológica">
          {#each mcYears as y (y.year)}
            {@const yearOpen = expandedYears.has(y.year)}
            <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
            <div class="mc-node mc-year" style="padding-left:.4rem" onclick={() => toggleYear(y.year)} role="button" tabindex="0" onkeydown={e => e.key === 'Enter' && toggleYear(y.year)}>
              <span class="mc-toggle">{yearOpen ? '▾' : '▸'}</span>
              <span class="mc-node-label">{y.year}</span>
            </div>
            {#if yearOpen}
              {#each y.items as m (m.id)}
                {@const label = memoryTimeLabel(m)}
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <div class="mc-node mc-memory {m.id === getActiveId() ? 'active' : ''}" style="padding-left:1.15rem" onclick={() => navigate(m.id)} role="button" tabindex="0" onkeydown={e => e.key === 'Enter' && navigate(m.id)}>
                  <i class="bx {m.icon || 'bx-calendar-event'} icon"></i>
                  {#if label}<span class="mc-time">{label}</span>{/if}
                  <span class="label">{m.title || 'Sem título'}</span>
                </div>
              {/each}
              {#each y.months as mo (mo.month)}
                {@const monthKey = `${y.year}-${mo.month}`}
                {@const monthOpen = !collapsedMonths.has(monthKey)}
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <div class="mc-node mc-month" style="padding-left:1.15rem" onclick={() => toggleMonth(monthKey)} role="button" tabindex="0" onkeydown={e => e.key === 'Enter' && toggleMonth(monthKey)}>
                  <span class="mc-toggle">{monthOpen ? '▾' : '▸'}</span>
                  <span class="mc-node-label">{['Janeiro','Fevereiro','Março','Abril','Maio','Junho','Julho','Agosto','Setembro','Outubro','Novembro','Dezembro'][mo.month - 1]}</span>
                </div>
                {#if monthOpen}
                  {#each mo.items as m (m.id)}
                    {@const label = memoryTimeLabel(m)}
                    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                    <div class="mc-node mc-memory {m.id === getActiveId() ? 'active' : ''}" style="padding-left:1.9rem" onclick={() => navigate(m.id)} role="button" tabindex="0" onkeydown={e => e.key === 'Enter' && navigate(m.id)}>
                      <i class="bx {m.icon || 'bx-calendar-event'} icon"></i>
                      {#if label}<span class="mc-time">{label}</span>{/if}
                      <span class="label">{m.title || 'Sem título'}</span>
                    </div>
                  {/each}
                  {#each mo.days as d (d.day)}
                    {@const dayKey = `${monthKey}-${d.day}`}
                    {@const dayOpen = !collapsedDays.has(dayKey)}
                    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                    <div class="mc-node mc-day" style="padding-left:1.9rem" onclick={() => toggleDay(dayKey)} role="button" tabindex="0" onkeydown={e => e.key === 'Enter' && toggleDay(dayKey)}>
                      <span class="mc-toggle">{dayOpen ? '▾' : '▸'}</span>
                      <span class="mc-node-label">Dia {d.day}</span>
                    </div>
                    {#if dayOpen}
                      {#each d.items as m (m.id)}
                        {@const label = memoryTimeLabel(m)}
                        <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                        <div class="mc-node mc-memory {m.id === getActiveId() ? 'active' : ''}" style="padding-left:2.65rem" onclick={() => navigate(m.id)} role="button" tabindex="0" onkeydown={e => e.key === 'Enter' && navigate(m.id)}>
                          <i class="bx {m.icon || 'bx-calendar-event'} icon"></i>
                          {#if label}<span class="mc-time">{label}</span>{/if}
                          <span class="label">{m.title || 'Sem título'}</span>
                        </div>
                      {/each}
                    {/if}
                  {/each}
                {/if}
              {/each}
            {/if}
          {/each}
          {#if mcYears.length === 0}
            <p class="tree-empty">Nenhuma memória ainda.</p>
          {/if}
        </nav>
        <div class="mc-new-row">
          <button class="new-doc-btn" onclick={() => newMemoryOpen = true}>
            + Nova Memória
          </button>
        </div>
      {/if}
    </div>
  {/if}

  <!-- New root document -->
  <div class="new-root">
    <button class="new-doc-btn" onclick={handleNewRoot}>
      + Novo documento
    </button>
  </div>
</div>

{#if newMemoryOpen}
  <NewMemoryDialog onClose={() => newMemoryOpen = false} onCreated={handleMemoryCreated} />
{/if}

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

  .tag-section {
    border-bottom: 1px solid var(--border);
  }

  .tag-section-header {
    display: flex;
    align-items: center;
    width: 100%;
    padding: .3rem .75rem;
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    font-size: .72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .04em;
  }

  .tag-section-header:hover { background: var(--bg-hover); }

  .tag-section-label { flex: 1; text-align: left; }

  .tag-section-arrow { font-size: .65rem; }

  .tag-filter {
    display: flex;
    flex-wrap: wrap;
    gap: .375rem;
    padding: .25rem .75rem .5rem;
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

  .mc-section {
    border-bottom: 1px solid var(--border);
  }

  .mc-tree {
    max-height: 40vh;
    overflow-y: auto;
  }

  .mc-node {
    display: flex;
    align-items: center;
    gap: .35rem;
    padding: .3rem .5rem .3rem 0;
    font-size: .875rem;
    color: var(--text);
    cursor: pointer;
    border-radius: var(--radius);
  }

  .mc-node:hover { background: var(--bg-hover); }

  .mc-toggle {
    width: 14px;
    flex-shrink: 0;
    font-size: .65rem;
    color: var(--text-muted);
  }

  .mc-node-label { flex: 1; }

  .mc-memory .icon {
    flex-shrink: 0;
    font-size: .95rem;
    color: var(--text-muted);
  }

  .mc-memory .label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mc-memory.active { background: var(--accent); color: #fff; }
  .mc-memory.active .icon,
  .mc-memory.active .mc-time { color: rgba(255,255,255,.85); }

  .mc-time {
    flex-shrink: 0;
    font-size: .72rem;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }

  .mc-new-row {
    padding: .3rem .5rem .5rem;
  }
</style>
