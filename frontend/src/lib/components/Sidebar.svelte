<script>
  import { onMount } from 'svelte'
  import TreeNode from './TreeNode.svelte'
  import { tree, loadTree, createDoc } from '../stores/documents.js'
  import { tags, loadTags } from '../stores/tags.js'

  let { onNavigate } = $props()

  let selectedTags = $state([])
  let currentHash = $state(window.location.hash)

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
</script>

<div class="sidebar-inner">
  <!-- Tag filters -->
  {#if $tags.length > 0}
    <div class="tag-filter" aria-label="Filtrar por tag">
      {#each $tags as tag}
        <button
          class="tag-chip {selectedTags.includes(tag.name) ? 'active' : ''}"
          onclick={() => toggleTag(tag.name)}
          title="#{tag.name} — {tag.count} documentos"
        >
          #{tag.name}
        </button>
      {/each}
    </div>
  {/if}

  <!-- Document tree -->
  <nav aria-label="Árvore de documentos" class="tree-nav">
    {#each $tree as node}
      <TreeNode {node} activeId={getActiveId()} {navigate} onNavigate={navigate} />
    {/each}
    {#if $tree.length === 0}
      <p class="tree-empty">Nenhum documento ainda.</p>
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
