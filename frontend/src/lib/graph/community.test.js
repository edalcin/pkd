import { test } from 'node:test'
import assert from 'node:assert/strict'
import { detectCommunities } from './community.js'

test('separa dois clusters densos ligados por aresta fraca', () => {
  // Triângulo A: 1-2-3 ; Triângulo B: 4-5-6 ; ponte fraca 3-4
  const nodeIds = [1, 2, 3, 4, 5, 6]
  const edges = [
    { source: 1, target: 2, weight: 1 },
    { source: 2, target: 3, weight: 1 },
    { source: 1, target: 3, weight: 1 },
    { source: 4, target: 5, weight: 1 },
    { source: 5, target: 6, weight: 1 },
    { source: 4, target: 6, weight: 1 },
    { source: 3, target: 4, weight: 0.1 },
  ]
  const c = detectCommunities(nodeIds, edges)
  assert.equal(c.get(1), c.get(2))
  assert.equal(c.get(2), c.get(3))
  assert.equal(c.get(4), c.get(5))
  assert.equal(c.get(5), c.get(6))
  assert.notEqual(c.get(1), c.get(4))
})

test('sem arestas -> cada nó é sua própria comunidade', () => {
  const c = detectCommunities([1, 2, 3], [])
  assert.equal(new Set([c.get(1), c.get(2), c.get(3)]).size, 3)
})
