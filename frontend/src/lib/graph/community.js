// Louvain local-moving, nível único, sobre grafo não-dirigido ponderado.
// ponytail: nível único (sem passes de agregação); adicionar agregação se os
// clusters ficarem grossos demais. Suficiente p/ grafo kNN de KB pessoal.
//
// nodeIds: array de ids (number). edges: [{source, target, weight}] (ids crus).
// Retorna Map<id, communityId> com rótulos densos 0..K-1.
export function detectCommunities(nodeIds, edges) {
  const adj = new Map()
  const k = new Map()
  for (const id of nodeIds) { adj.set(id, []); k.set(id, 0) }
  let m2 = 0
  for (const e of edges) {
    if (!adj.has(e.source) || !adj.has(e.target) || e.source === e.target) continue
    const w = e.weight || 1
    adj.get(e.source).push({ n: e.target, w })
    adj.get(e.target).push({ n: e.source, w })
    k.set(e.source, k.get(e.source) + w)
    k.set(e.target, k.get(e.target) + w)
    m2 += 2 * w
  }
  const comm = new Map()
  nodeIds.forEach(id => comm.set(id, id))
  if (m2 === 0) return relabel(nodeIds, comm)
  const sigTot = new Map()
  nodeIds.forEach(id => sigTot.set(id, k.get(id)))
  let improved = true, iters = 0
  while (improved && iters++ < 20) {
    improved = false
    for (const id of nodeIds) {
      const ki = k.get(id)
      const cur = comm.get(id)
      sigTot.set(cur, sigTot.get(cur) - ki)
      const wTo = new Map()
      for (const { n, w } of adj.get(id)) {
        const c = comm.get(n)
        wTo.set(c, (wTo.get(c) || 0) + w)
      }
      let best = cur, bestGain = (wTo.get(cur) || 0) - (sigTot.get(cur) || 0) * ki / m2
      for (const [c, wic] of wTo) {
        const gain = wic - (sigTot.get(c) || 0) * ki / m2
        if (gain > bestGain) { bestGain = gain; best = c }
      }
      comm.set(id, best)
      sigTot.set(best, (sigTot.get(best) || 0) + ki)
      if (best !== cur) improved = true
    }
  }
  return relabel(nodeIds, comm)
}

function relabel(nodeIds, comm) {
  const dense = new Map()
  const out = new Map()
  for (const id of nodeIds) {
    const c = comm.get(id)
    if (!dense.has(c)) dense.set(c, dense.size)
    out.set(id, dense.get(c))
  }
  return out
}

// Cor distinta por índice de comunidade via ângulo dourado (sem paleta fixa).
export function communityColor(i) {
  return `hsl(${(i * 137.508) % 360}, 62%, 58%)`
}
