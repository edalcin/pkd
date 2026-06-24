<script>
  let { node, depth = 0 } = $props()
  let expanded = $state(true)

  function open() {
    window.location.hash = `/doc/${node.id}`
  }
</script>

<div>
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div
    class="tree-item"
    style="padding-left: {0.4 + depth * 0.75}rem"
    onclick={open}
    role="button"
    tabindex="0"
    onkeydown={e => e.key === 'Enter' && open()}
  >
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

    <i class="bx {node.icon || 'bx-file-blank'} icon"></i>
    <span class="label">{node.title || 'Sem título'}</span>
  </div>

  {#if expanded && node.children?.length}
    <div>
      {#each node.children as child (child.id)}
        <svelte:self node={child} depth={depth + 1} />
      {/each}
    </div>
  {/if}
</div>

<style>
  .toggle-btn {
    width: 14px;
    font-size: .65rem;
    color: var(--text-muted);
    flex-shrink: 0;
    cursor: pointer;
    background: none;
    border: none;
    padding: 0;
    line-height: 1;
  }
</style>
