<script>
  import { onMount, onDestroy, untrack } from 'svelte'
  import { forceSimulation, forceLink, forceManyBody, forceCenter, forceCollide, forceX, forceY } from 'd3-force'
  import { select } from 'd3-selection'
  import { zoom, zoomIdentity } from 'd3-zoom'
  import { drag } from 'd3-drag'
  import { apiGet } from '../api.js'
  import { textFilter, tree } from '../stores/documents.js'
  import { detectCommunities, communityColor } from '../graph/community.js'

  let svgEl = $state(null)
  let rawNodes = $state([])
  let rawEdges = $state([])
  let nodes = $state([])
  let links = $state([])
  let loading = $state(true)
  let showAll = $state(false)
  let showHierarchy = $state(true)
  let showLinks = $state(true)
  let showTagEdges = $state(true)
  let showSemantic = $state(false)
  let semanticEdges = $state([])
  let semanticLoading = $state(false)
  let semanticAvailable = $state(true)
  let tagFilter = $state('')
  let simulation = null
  let communityCount = $state(0)
  let communityMeta = $state(new Map())
  let selectedCommunities = $state(new Set())
  let editingCommId = $state(null)

  // IDs of docs currently in the tree (mirrors sidebar FTS5 filter result)
  const treeDocIds = $derived.by(() => {
    const ids = new Set()
    function collect(nodes) {
      for (const n of nodes) {
        ids.add(n.id)
        if (n.children?.length) collect(n.children)
      }
    }
    collect($tree)
    return ids
  })

  // Derived count of matching doc nodes (for empty-state check, evaluated before effect)
  const filteredDocCount = $derived.by(() => {
    if (!$textFilter) return rawNodes.filter(n => n.node_type !== 'tag').length
    return rawNodes.filter(n => n.node_type !== 'tag' && treeDocIds.has(n.id)).length
  })

  // Tag color palette
  const COLORS = [
    '#818cf8', '#34d399', '#f59e0b', '#60a5fa',
    '#f472b6', '#a78bfa', '#2dd4bf', '#fb923c',
  ]
  const tagColorMap = new Map()
  let colorIdx = 0

  function getColor(tags) {
    const primary = tags?.[0]
    if (!primary) return '#94a3b8'
    if (!tagColorMap.has(primary)) {
      tagColorMap.set(primary, COLORS[colorIdx++ % COLORS.length])
    }
    return tagColorMap.get(primary)
  }

  onMount(() => loadGraph())
  onDestroy(() => simulation?.stop())

  // Re-renders whenever rawNodes, svgEl, or any toggle changes.
  // getFilteredData() reads all toggle $state vars so they become tracked dependencies.
  $effect(() => {
    if (svgEl && rawNodes.length > 0) {
      const { nodes: fn, edges: fe } = getFilteredData()
      setupGraph(fn, fe)
    }
  })

  // Toggles D3 node/edge visibility based on selectedCommunities.
  function applyVisibility() {
    if (!svgEl) return
    const sel = selectedCommunities
    select(svgEl).selectAll('.graph-node')
      .attr('display', d => sel.size > 0 && d.community !== undefined && !sel.has(d.community) ? 'none' : null)
    select(svgEl).selectAll('line')
      .attr('display', d => {
        if (sel.size === 0) return null
        const sc = typeof d.source === 'object' ? d.source.community : undefined
        const tc = typeof d.target === 'object' ? d.target.community : undefined
        if (sc === undefined || tc === undefined) return null
        return sel.has(sc) && sel.has(tc) ? null : 'none'
      })
  }

  // Re-apply visibility whenever selectedCommunities changes.
  $effect(() => { applyVisibility() })

  // Init selectedCommunities to all community IDs when communityMeta changes.
  $effect(() => {
    const ids = new Set(communityMeta.keys())
    if (ids.size === 0) { selectedCommunities = new Set(); return }
    const cur = untrack(() => selectedCommunities)
    const hasOverlap = [...ids].some(id => cur.has(id))
    if (!hasOverlap) selectedCommunities = ids
  })

  // Returns the subset of nodes and edges to render based on active toggles and text filter.
  function getFilteredData() {
    const q = $textFilter.toLowerCase().trim()

    if (!q) {
      const sourceEdges = showSemantic ? [...rawEdges, ...semanticEdges] : rawEdges
      const visibleEdges = sourceEdges.filter(e => {
        if (e.edge_type === 'link') return showLinks
        if (e.edge_type === 'tag') return showTagEdges
        if (e.edge_type === 'hierarchy') return showHierarchy
        if (e.edge_type === 'semantic') return showSemantic
        return true
      })
      if (showAll) return { nodes: rawNodes, edges: visibleEdges }
      const connectedIDs = new Set()
      for (const e of visibleEdges) {
        connectedIDs.add(e.source)
        connectedIDs.add(e.target)
      }
      return {
        nodes: rawNodes.filter(n => connectedIDs.has(n.id)),
        edges: visibleEdges,
      }
    }

    // Text filter: use same doc set as sidebar (FTS5 result via tree store)
    const matchIds = treeDocIds

    const sourceEdges = showSemantic ? [...rawEdges, ...semanticEdges] : rawEdges
    const visibleEdges = sourceEdges.filter(e => {
      if (e.edge_type === 'semantic' && !showSemantic) return false
      if (e.edge_type === 'link' && !showLinks) return false
      if (e.edge_type === 'tag' && !showTagEdges) return false
      if (e.edge_type === 'hierarchy' && !showHierarchy) return false
      // tag edges: keep if the doc side matches
      if (e.edge_type === 'tag') return matchIds.has(e.source) || matchIds.has(e.target)
      // doc-doc edges: both endpoints must match
      return matchIds.has(e.source) && matchIds.has(e.target)
    })

    // Always include matching docs (even isolated) plus nodes reachable via edges
    const visibleIds = new Set(matchIds)
    for (const e of visibleEdges) {
      visibleIds.add(e.source)
      visibleIds.add(e.target)
    }

    return {
      nodes: rawNodes.filter(n => visibleIds.has(n.id)),
      edges: visibleEdges,
    }
  }

  async function loadGraph() {
    loading = true
    simulation?.stop()
    try {
      const params = new URLSearchParams()
      if (showAll || showSemantic) params.set('all', 'true')
      if (tagFilter) params.set('tag', tagFilter)
      const data = await apiGet('/api/graph?' + params)
      rawNodes = data.nodes || []
      rawEdges = data.edges || []
    } finally {
      loading = false
    }
  }

  async function toggleSemantic() {
    if (showSemantic && semanticEdges.length === 0) {
      semanticLoading = true
      try {
        const data = await apiGet('/api/graph/semantic')
        semanticEdges = data.edges || []
      } catch (e) {
        showSemantic = false
        semanticAvailable = false
        semanticLoading = false
        return
      }
      semanticLoading = false
    }
    await loadGraph() // refetch: todos-os-docs quando ligado, conectados quando desligado
  }

  function setupGraph(initNodes, initEdges) {
    if (!svgEl) return

    const width = svgEl.clientWidth
    const height = svgEl.clientHeight

    // Clear previous render
    select(svgEl).selectAll('*').remove()

    const svg = select(svgEl)

    // Arrowhead marker for hierarchy edges (parent→child direction)
    svg.append('defs').append('marker')
      .attr('id', 'arrowhead')
      .attr('viewBox', '0 -5 10 10')
      .attr('refX', 20)  // tip(10) + node-radius(6px / 0.6scale = 10 units)
      .attr('refY', 0)
      .attr('orient', 'auto')
      .attr('markerWidth', 6)
      .attr('markerHeight', 6)
      .append('path')
      .attr('d', 'M0,-5L10,0L0,5')
      .attr('fill', 'var(--accent)')

    const g = svg.append('g')

    // Zoom behavior
    const zoomBehavior = zoom()
      .scaleExtent([.1, 5])
      .on('zoom', e => g.attr('transform', e.transform))
    svg.call(zoomBehavior)

    // Mutable copies for D3 (spread preserves edge_type and other fields)
    const ns = initNodes.map(n => ({ ...n }))
    const ls = initEdges.map(e => ({
      ...e,
      source: ns.find(n => n.id === e.source) || e.source,
      target: ns.find(n => n.id === e.target) || e.target,
    }))

    // Comunidades (só no modo semântico): agrupa docs pelas arestas semânticas.
    let clusterCenters = null
    if (showSemantic) {
      const semEdges = ls
        .filter(e => e.edge_type === 'semantic')
        .map(e => ({
          source: e.source.id ?? e.source,
          target: e.target.id ?? e.target,
          weight: e.weight,
        }))
      const idToComm = detectCommunities(ns.map(n => n.id), semEdges)
      ns.forEach(n => { n.community = idToComm.get(n.id) ?? 0 })
      const commSize = new Map()
      ns.forEach(n => commSize.set(n.community, (commSize.get(n.community) || 0) + 1))
      const K = commSize.size
      communityCount = [...commSize.values()].filter(s => s > 1).length
      const R = Math.min(width, height) * 0.35
      clusterCenters = Array.from({ length: K }, (_, idx) => {
        const a = (idx / K) * 2 * Math.PI
        return { x: width / 2 + R * Math.cos(a), y: height / 2 + R * Math.sin(a) }
      })
      // semente perto do centro da comunidade -> arranjo inicial já agrupado
      ns.forEach(n => {
        const c = clusterCenters[n.community] || { x: width / 2, y: height / 2 }
        n.x = c.x + (Math.random() - 0.5) * 60
        n.y = c.y + (Math.random() - 0.5) * 60
      })
      ns._commSize = commSize // usado na coloração abaixo
      // Populate communityMeta for legend (skip singletons).
      const newMeta = new Map()
      for (const [commId, size] of commSize) {
        if (size <= 1) continue
        const commNodes = ns.filter(n => n.community === commId)
        const nodeIds = commNodes.map(n => n.id).sort((a, b) => a - b)
        const key = nodeIds.join(',')
        const stored = localStorage.getItem(`pkd:comm:${key}`)
        newMeta.set(commId, {
          key,
          name: stored || `Comunidade ${newMeta.size + 1}`,
          nodeIds,
          titles: commNodes.map(n => n.title),
          size,
        })
      }
      communityMeta = newMeta
    } else {
      communityCount = 0
      communityMeta = new Map()
    }

    // Edges
    const edgeEls = g.append('g')
      .selectAll('line')
      .data(ls)
      .enter().append('line')
      .attr('class', d => {
        if (d.edge_type === 'hierarchy') return 'graph-edge graph-edge--hierarchy'
        if (d.edge_type === 'tag') return 'graph-edge graph-edge--tag'
        if (d.edge_type === 'semantic') return 'graph-edge graph-edge--semantic'
        return 'graph-edge graph-edge--link'
      })
      .attr('stroke-width', d => d.edge_type === 'semantic' ? 0.5 + (d.weight || 0) * 2 : 1.5)
      .attr('marker-end', d => d.edge_type === 'hierarchy' ? 'url(#arrowhead)' : null)

    // Track drag vs click
    let didDrag = false

    const dragBehavior = drag()
      .on('start', function(event, d) {
        didDrag = false
        if (!event.active) simulation.alphaTarget(0.3).restart()
        d.fx = d.x
        d.fy = d.y
        select(this).attr('cursor', 'grabbing')
      })
      .on('drag', function(event, d) {
        didDrag = true
        d.fx = event.x
        d.fy = event.y
      })
      .on('end', function(event, d) {
        if (!event.active) simulation.alphaTarget(0)
        if (didDrag) {
          // thicker stroke = visually pinned; dblclick resets to 2
          select(this).select('circle').attr('stroke-width', 3.5)
        }
        select(this).attr('cursor', 'grab')
      })

    // Nodes group
    const nodeEls = g.append('g')
      .selectAll('g')
      .data(ns)
      .enter().append('g')
      .attr('class', 'graph-node')
      .attr('cursor', 'grab')
      .call(dragBehavior)
      .on('click', (_, d) => {
        if (didDrag) { didDrag = false; return }
        if (d.node_type === 'tag') {
          tagFilter = d.title.replace(/^#/, '')
          loadGraph()
        } else {
          window.location.hash = `/doc/${d.id}`
        }
      })
      .on('dblclick', (event, d) => {
        event.stopPropagation()
        d.fx = null
        d.fy = null
        simulation.alphaTarget(0.1).restart()
        select(event.currentTarget).select('circle')
          .attr('stroke-width', 2)
      })

    nodeEls.append('circle')
      .attr('r', d => d.node_type === 'tag' ? 7 : 6)
      .attr('fill', d => d.node_type === 'tag'
        ? '#e879f9'
        : showSemantic
          ? ((ns._commSize?.get(d.community) || 0) > 1 ? communityColor(d.community) : '#94a3b8')
          : getColor(d.tags))
      .attr('stroke', d => d.node_type === 'tag' ? '#c026d3' : 'var(--bg-panel)')
      .attr('stroke-width', 2)
      .attr('stroke-dasharray', d => d.node_type === 'tag' ? '3,2' : 'none')

    nodeEls.append('text')
      .attr('dy', 14)
      .attr('text-anchor', 'middle')
      .text(d => d.title.length > 20 ? d.title.slice(0, 18) + '…' : d.title)

    // Tooltip on hover
    nodeEls
      .on('mouseenter', function(_, d) {
        select(this).select('circle').attr('r', d.node_type === 'tag' ? 9 : 9)
      })
      .on('mouseleave', function(_, d) {
        select(this).select('circle').attr('r', d.node_type === 'tag' ? 7 : 6)
      })

    // Force simulation
    simulation = forceSimulation(ns)
      .force('link', forceLink(ls).distance(showSemantic ? 60 : 80).strength(.5))
      .force('charge', forceManyBody().strength(showSemantic ? -80 : -120))
      .force('collide', forceCollide(18))
    if (showSemantic && clusterCenters) {
      simulation
        .force('x', forceX(d => (clusterCenters[d.community] || { x: width / 2 }).x).strength(0.18))
        .force('y', forceY(d => (clusterCenters[d.community] || { y: height / 2 }).y).strength(0.18))
    } else {
      simulation.force('center', forceCenter(width / 2, height / 2))
    }
    simulation
      .on('tick', () => {
        edgeEls
          .attr('x1', d => d.source.x)
          .attr('y1', d => d.source.y)
          .attr('x2', d => d.target.x)
          .attr('y2', d => d.target.y)
        nodeEls.attr('transform', d => `translate(${d.x},${d.y})`)
      })

    untrack(() => applyVisibility())

    nodes = ns
    links = ls
  }

  function zoomToFit() {
    if (!svgEl) return
    select(svgEl).transition().duration(500)
      .call(zoom().transform, zoomIdentity)
  }

  function toggleCommunity(commId) {
    const next = new Set(selectedCommunities)
    if (next.has(commId)) next.delete(commId)
    else next.add(commId)
    selectedCommunities = next
  }

  function toggleAllCommunities() {
    selectedCommunities = selectedCommunities.size === communityMeta.size
      ? new Set()
      : new Set(communityMeta.keys())
  }

  function saveCommunityName(commId, name) {
    const meta = communityMeta.get(commId)
    if (!meta) { editingCommId = null; return }
    const trimmed = name.trim() || meta.name
    localStorage.setItem(`pkd:comm:${meta.key}`, trimmed)
    const m = new Map(communityMeta)
    m.set(commId, { ...meta, name: trimmed })
    communityMeta = m
    editingCommId = null
  }

</script>

<div class="graph-container">
  {#if loading}
    <div class="empty-state"><div class="spinner"></div></div>
  {:else if rawNodes.length === 0}
    <div class="empty-state">
      <span class="emoji">🕸️</span>
      <p>Nenhuma conexão ainda.<br>Adicione tags ou relacione documentos no editor.</p>
      {#if !showAll}
        <button class="btn btn-ghost" onclick={() => { showAll = true; loadGraph() }}>
          Mostrar todos os documentos
        </button>
      {/if}
    </div>
  {:else if $textFilter && filteredDocCount === 0}
    <div class="empty-state">
      <span class="emoji">🔍</span>
      <p>Nenhum documento encontrado para "<strong>{$textFilter}</strong>".</p>
    </div>
  {:else}
    <!-- Controls -->
    <div class="graph-controls">
      <input
        class="tag-filter-input"
        type="text"
        placeholder="Filtrar por tag…"
        bind:value={tagFilter}
        oninput={loadGraph}
      />
      <label class="show-all-toggle">
        <input type="checkbox" bind:checked={showAll} onchange={loadGraph} />
        Todos os docs
      </label>
      <label class="show-all-toggle">
        <input type="checkbox" bind:checked={showHierarchy} />
        Hierarquia
      </label>
      <label class="show-all-toggle">
        <input type="checkbox" bind:checked={showLinks} />
        Links entre docs
      </label>
      <label class="show-all-toggle">
        <input type="checkbox" bind:checked={showTagEdges} />
        Relações com tags
      </label>
      {#if semanticAvailable}
        <label class="show-all-toggle">
          <input type="checkbox" bind:checked={showSemantic} onchange={toggleSemantic} disabled={semanticLoading} />
          Semântico{#if semanticLoading} …{/if}
        </label>
      {/if}
      <button class="btn btn-ghost" onclick={zoomToFit} title="Ajustar tela">⤢ Ajustar</button>
      <span class="node-count">{nodes.length} nós · {links.length} arestas{#if showSemantic} · {communityCount} comunidades{/if}</span>
    </div>

    <svg
      bind:this={svgEl}
      class="graph-svg"
      aria-label="Grafo de conexões entre documentos"
    ></svg>

    <div class="graph-legend">
      <div class="legend-title">Legenda</div>
      <div class="legend-section-label">Nós</div>
      <div class="legend-item">
        <svg width="14" height="14"><circle cx="7" cy="7" r="6" fill="#94a3b8" stroke="var(--bg-panel)" stroke-width="2"/></svg>
        <span>Documento</span>
      </div>
      <div class="legend-item">
        <svg width="14" height="14">
          {#each [0,1,2] as i}
            <circle cx={3 + i * 5} cy="7" r="3" fill={['#818cf8','#34d399','#f59e0b'][i]}/>
          {/each}
        </svg>
        <span>Doc. c/ tag</span>
      </div>
      <div class="legend-item">
        <svg width="14" height="14"><circle cx="7" cy="7" r="6" fill="#e879f9" stroke="#c026d3" stroke-width="2" stroke-dasharray="3,2"/></svg>
        <span>Tag</span>
      </div>
      <div class="legend-section-label">Arestas</div>
      <div class="legend-item">
        <svg width="22" height="10"><line x1="1" y1="5" x2="21" y2="5" stroke="#60a5fa" stroke-width="1.5" stroke-opacity=".8"/></svg>
        <span>Link</span>
      </div>
      <div class="legend-item">
        <svg width="22" height="10"><line x1="1" y1="5" x2="21" y2="5" stroke="var(--accent)" stroke-width="1.5" stroke-opacity=".8" stroke-dasharray="6,3"/></svg>
        <span>Hierarquia</span>
      </div>
      <div class="legend-item">
        <svg width="22" height="10"><line x1="1" y1="5" x2="21" y2="5" stroke="#e879f9" stroke-width="1.5" stroke-opacity=".7" stroke-dasharray="2,3"/></svg>
        <span>Rel. com tag</span>
      </div>
      <div class="legend-item">
        <svg width="22" height="10"><line x1="1" y1="5" x2="21" y2="5" stroke="#a78bfa" stroke-width="1.5" stroke-opacity=".8" stroke-dasharray="3,2"/></svg>
        <span>Semântico</span>
      </div>
      {#if showSemantic && communityMeta.size > 0}
        <div class="legend-section-label">Comunidades</div>
        <label class="legend-item" style="gap:.3rem;cursor:pointer">
          <input type="checkbox" style="cursor:pointer"
            checked={selectedCommunities.size === communityMeta.size}
            onchange={toggleAllCommunities}
          />
          <span style="font-size:.72rem">Todas</span>
        </label>
        {#each [...communityMeta.entries()] as [commId, meta]}
          <div class="legend-item" style="align-items:flex-start;gap:.3rem">
            <input type="checkbox"
              style="cursor:pointer;margin-top:.15rem"
              checked={selectedCommunities.has(commId)}
              onchange={() => toggleCommunity(commId)}
            />
            <svg width="10" height="10" style="flex-shrink:0;margin-top:.1rem">
              <circle cx="5" cy="5" r="5" fill={communityColor(commId)} />
            </svg>
            <div style="flex:1;min-width:0">
              {#if editingCommId === commId}
                <input
                  class="comm-name-input"
                  type="text"
                  value={meta.name}
                  onblur={e => saveCommunityName(commId, e.currentTarget.value)}
                  onkeydown={e => e.key === 'Enter' && saveCommunityName(commId, e.currentTarget.value)}
                  autofocus
                />
              {:else}
                <button class="comm-name" onclick={() => { editingCommId = commId }} title="Clique para renomear">{meta.name}</button>
              {/if}
            </div>
            <span style="color:var(--text-muted);font-size:.7rem;white-space:nowrap">({meta.size})</span>
          </div>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .tag-filter-input {
    width: 140px;
    padding: .3rem .5rem;
    font-size: .8rem;
  }

  .show-all-toggle {
    display: flex;
    align-items: center;
    gap: .375rem;
    font-size: .8rem;
    cursor: pointer;
    color: var(--text-muted);
  }

  .node-count {
    font-size: .75rem;
    color: var(--text-muted);
    text-align: center;
  }

  .graph-legend {
    position: absolute;
    bottom: 1rem;
    left: 1rem;
    display: flex;
    flex-direction: column;
    gap: .3rem;
    background: var(--bg-panel);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: .5rem .625rem;
    box-shadow: var(--shadow);
  }

  .legend-title {
    font-size: .7rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: .05em;
    margin-bottom: .15rem;
  }

  .legend-section-label {
    font-size: .65rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: .04em;
    margin-top: .25rem;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: .4rem;
    font-size: .75rem;
    color: var(--text-muted);
  }

  .legend-item svg {
    flex-shrink: 0;
  }

  .comm-name {
    background: none;
    border: none;
    padding: 0;
    cursor: text;
    font-size: .75rem;
    word-break: break-word;
    color: var(--text-muted);
    text-align: left;
  }
  .comm-name-input {
    width: 90px;
    padding: .1rem .25rem;
    font-size: .72rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm, 4px);
    background: var(--bg-input, var(--bg-panel));
    color: var(--text);
  }
</style>

