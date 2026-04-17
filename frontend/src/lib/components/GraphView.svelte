<script>
  import { onMount, onDestroy } from 'svelte'
  import { forceSimulation, forceLink, forceManyBody, forceCenter, forceCollide } from 'd3-force'
  import { select } from 'd3-selection'
  import { zoom, zoomIdentity } from 'd3-zoom'
  import { apiGet } from '../api.js'

  let svgEl = $state(null)
  let rawNodes = $state([])
  let rawEdges = $state([])
  let nodes = $state([])
  let links = $state([])
  let loading = $state(true)
  let showAll = $state(false)
  let tagFilter = $state('')
  let simulation = null

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

  // Fire setupGraph only after svgEl is in the DOM and data is ready.
  $effect(() => {
    if (svgEl && rawNodes.length > 0) {
      setupGraph(rawNodes, rawEdges)
    }
  })

  async function loadGraph() {
    loading = true
    simulation?.stop()
    try {
      const params = new URLSearchParams()
      if (showAll) params.set('all', 'true')
      if (tagFilter) params.set('tag', tagFilter)
      const data = await apiGet('/api/graph?' + params)
      rawNodes = data.nodes || []
      rawEdges = data.edges || []
    } finally {
      loading = false
    }
  }

  function setupGraph(rawNodes, rawEdges) {
    if (!svgEl) return

    const width = svgEl.clientWidth
    const height = svgEl.clientHeight

    // Clear previous render
    select(svgEl).selectAll('*').remove()

    const svg = select(svgEl)
    const g = svg.append('g')

    // Zoom behavior
    const zoomBehavior = zoom()
      .scaleExtent([.1, 5])
      .on('zoom', e => g.attr('transform', e.transform))
    svg.call(zoomBehavior)

    // Mutable copies for D3
    const ns = rawNodes.map(n => ({ ...n }))
    const ls = rawEdges.map(e => ({
      source: ns.find(n => n.id === e.source) || e.source,
      target: ns.find(n => n.id === e.target) || e.target,
    }))

    // Edges
    const edgeEls = g.append('g')
      .selectAll('line')
      .data(ls)
      .enter().append('line')
      .attr('class', 'graph-edge')
      .attr('stroke-width', 1.5)

    // Nodes group
    const nodeEls = g.append('g')
      .selectAll('g')
      .data(ns)
      .enter().append('g')
      .attr('class', 'graph-node')
      .attr('cursor', 'pointer')
      .on('click', (_, d) => {
        if (d.node_type === 'tag') {
          tagFilter = d.title.replace(/^#/, '')
          loadGraph()
        } else {
          window.location.hash = `/doc/${d.id}`
        }
      })

    nodeEls.append('circle')
      .attr('r', d => d.node_type === 'tag' ? 7 : 6)
      .attr('fill', d => d.node_type === 'tag' ? '#e879f9' : getColor(d.tags))
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
      .force('link', forceLink(ls).distance(80).strength(.5))
      .force('charge', forceManyBody().strength(-120))
      .force('center', forceCenter(width / 2, height / 2))
      .force('collide', forceCollide(18))
      .on('tick', () => {
        edgeEls
          .attr('x1', d => d.source.x)
          .attr('y1', d => d.source.y)
          .attr('x2', d => d.target.x)
          .attr('y2', d => d.target.y)
        nodeEls.attr('transform', d => `translate(${d.x},${d.y})`)
      })

    nodes = ns
    links = ls
  }

  function zoomToFit() {
    if (!svgEl) return
    select(svgEl).transition().duration(500)
      .call(zoom().transform, zoomIdentity)
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
      <button class="btn btn-ghost" onclick={zoomToFit} title="Ajustar tela">⤢ Ajustar</button>
      <span class="node-count">{nodes.length} nós · {links.length} arestas</span>
    </div>

    <svg
      bind:this={svgEl}
      class="graph-svg"
      aria-label="Grafo de conexões entre documentos"
    ></svg>
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
</style>
