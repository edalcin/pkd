<script>
  import { MONTH_NAMES, PERIOD_LABELS } from '../stores/memories.js'

  let {
    year = $bindable(null),
    month = $bindable(null),
    day = $bindable(null),
    momento = $bindable('nenhum'), // 'nenhum' | 'hora' | 'periodo'
    time = $bindable(''),          // "HH:MM", for the 'hora' radio
    period = $bindable(''),        // period code, for the 'periodo' radio
    disabled = false,
    focusDay = false,              // autofocus the Dia field once it mounts enabled
    onchange = null,
  } = $props()

  const uid = Math.random().toString(36).slice(2)
  let dayEl = $state(null)

  function daysInMonth(y, m) {
    if (!y || !m) return 31
    return new Date(y, m, 0).getDate()
  }

  // Clearing a coarser field clears everything finer than it.
  $effect(() => { if (!month) day = null })
  $effect(() => { if (!day) { momento = 'nenhum'; time = ''; period = '' } })

  $effect(() => {
    if (focusDay && dayEl && !disabled && month) setTimeout(() => dayEl?.focus(), 30)
  })

  function fire() { onchange?.() }
</script>

<div class="memdate-row">
  <label class="memdate-field">
    <span class="memdate-label">Ano *</span>
    <input
      class="memdate-input memdate-year"
      type="number"
      min="1" max="9999"
      bind:value={year}
      onblur={fire}
      {disabled}
      aria-label="Ano"
      required
    />
  </label>

  <label class="memdate-field">
    <span class="memdate-label">Mês</span>
    <select class="memdate-input" bind:value={month} onchange={fire} disabled={disabled || !year} aria-label="Mês">
      <option value={null}>—</option>
      {#each MONTH_NAMES as m, i}
        <option value={i + 1}>{m}</option>
      {/each}
    </select>
  </label>

  <label class="memdate-field">
    <span class="memdate-label">Dia</span>
    <select
      class="memdate-input"
      bind:value={day}
      bind:this={dayEl}
      onchange={fire}
      disabled={disabled || !month}
      aria-label="Dia"
    >
      <option value={null}>—</option>
      {#each Array.from({ length: daysInMonth(year, month) }, (_, i) => i + 1) as d}
        <option value={d}>{d}</option>
      {/each}
    </select>
  </label>
</div>

<div class="memdate-momento">
  <span class="memdate-label">Momento do dia</span>
  <div class="memdate-radios">
    <label class="memdate-radio">
      <input type="radio" name="momento-{uid}" value="nenhum" bind:group={momento} disabled={disabled || !day} onchange={fire} />
      nenhum
    </label>
    <label class="memdate-radio">
      <input type="radio" name="momento-{uid}" value="hora" bind:group={momento} disabled={disabled || !day} onchange={fire} />
      hora exata
      <input
        type="time"
        class="memdate-time"
        bind:value={time}
        disabled={disabled || !day || momento !== 'hora'}
        onchange={fire}
        aria-label="Hora"
      />
    </label>
    <label class="memdate-radio">
      <input type="radio" name="momento-{uid}" value="periodo" bind:group={momento} disabled={disabled || !day} onchange={fire} />
      Período
      <select
        class="memdate-input"
        bind:value={period}
        disabled={disabled || !day || momento !== 'periodo'}
        onchange={fire}
        aria-label="Período"
      >
        <option value="">—</option>
        {#each Object.entries(PERIOD_LABELS) as [code, label]}
          <option value={code}>{label}</option>
        {/each}
      </select>
    </label>
  </div>
</div>

<style>
  .memdate-row {
    display: flex;
    gap: .5rem;
    margin-bottom: .75rem;
  }

  .memdate-field {
    display: flex;
    flex-direction: column;
    gap: .25rem;
    flex: 1;
  }

  .memdate-label {
    font-size: .75rem;
    color: var(--text-muted);
  }

  .memdate-input {
    padding: .4rem .5rem;
    font-size: .875rem;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    width: 100%;
  }

  .memdate-input:disabled { opacity: .5; cursor: not-allowed; }

  .memdate-momento { margin-bottom: .5rem; }

  .memdate-radios {
    display: flex;
    flex-direction: column;
    gap: .4rem;
    margin-top: .3rem;
  }

  .memdate-radio {
    display: flex;
    align-items: center;
    gap: .4rem;
    font-size: .8rem;
    color: var(--text);
  }

  .memdate-radio:has(input:disabled) { color: var(--text-muted); }

  .memdate-time {
    padding: .3rem .4rem;
    font-size: .8rem;
    border-radius: var(--radius);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
  }

  .memdate-time:disabled { opacity: .5; cursor: not-allowed; }
</style>
