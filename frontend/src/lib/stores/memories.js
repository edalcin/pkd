import { writable } from 'svelte/store'
import { apiGet, apiPost, apiPatch } from '../api.js'

/** Flat list of Memórias, already sorted by the server (year desc; month-less
 *  first inside a year, then months desc; day-less first inside a month,
 *  then days desc; chronological ascending inside a day). Group it
 *  consecutively for display — never re-sort. */
export const memories = writable([])

/** Reload the Memória Cronológica list from the server. */
export async function loadMemories() {
  memories.set(await apiGet('/api/memories'))
}

/** Create a new Memória. date: {year, month?, day?, hour?, minute?, period?} */
export async function createMemory(title, date, { content, tags } = {}) {
  const doc = await apiPost('/api/memories', { title, content, tags, date })
  await loadMemories()
  return doc
}

/** Replace a Memória's Data da Memória. memoryId is the public MEM-… id
 *  (never the numeric document id) — it is frozen at creation (ADR-007). */
export async function updateMemoryDate(memoryId, date) {
  const doc = await apiPatch(`/api/memories/${memoryId}`, { date })
  await loadMemories()
  return doc
}

export const MONTH_NAMES = [
  'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho',
  'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro',
]

/** Período code → pt-BR label. Never show the raw code in the UI. */
export const PERIOD_LABELS = {
  madrugada: 'Madrugada',
  manha: 'Manhã',
  almoco: 'Hora do almoço',
  tarde: 'Tarde',
  lanche: 'Hora do lanche',
  jantar: 'Hora do jantar',
  noite: 'Noite',
}

/** "14:30" or the Período label for a MemoryListItem / Document; '' if the
 *  Memória has no time component (year/month/day-only precision). */
export function memoryTimeLabel(m) {
  const hour = m.hour ?? m.memory_hour
  const minute = m.minute ?? m.memory_minute
  const period = m.period ?? m.memory_period
  if (hour != null) return `${String(hour).padStart(2, '0')}:${String(minute ?? 0).padStart(2, '0')}`
  if (period) return PERIOD_LABELS[period] || period
  return ''
}

/** Group the already-sorted flat Memória list into Ano → Mês → Dia, purely
 *  by consecutive runs (the server owns the order). Memórias without month
 *  live in the year's own `items`; without day, in the month's `items`. */
export function groupMemories(list) {
  const years = []
  let curYear = null, curMonth = null, curDay = null
  for (const m of list) {
    if (!curYear || curYear.year !== m.year) {
      curYear = { year: m.year, items: [], months: [] }
      years.push(curYear)
      curMonth = null
      curDay = null
    }
    if (m.month == null) { curYear.items.push(m); continue }
    if (!curMonth || curMonth.month !== m.month) {
      curMonth = { month: m.month, items: [], days: [] }
      curYear.months.push(curMonth)
      curDay = null
    }
    if (m.day == null) { curMonth.items.push(m); continue }
    if (!curDay || curDay.day !== m.day) {
      curDay = { day: m.day, items: [] }
      curMonth.days.push(curDay)
    }
    curDay.items.push(m)
  }
  return years
}

/** Build the {date} payload for POST/PATCH from MemoryDateFields' flat state. */
export function buildMemoryDate({ year, month, day, momento, time, period }) {
  const date = { year: Number(year) }
  if (month) date.month = Number(month)
  if (month && day) date.day = Number(day)
  if (month && day && momento === 'hora' && time) {
    const [h, min] = time.split(':').map(Number)
    date.hour = h
    date.minute = min
  } else if (month && day && momento === 'periodo' && period) {
    date.period = period
  }
  return date
}

/** Derive MemoryDateFields' flat state from a loaded Document (memory_id set). */
export function memoryDateFromDoc(doc) {
  const momento = doc.memory_hour != null ? 'hora' : doc.memory_period ? 'periodo' : 'nenhum'
  const time = doc.memory_hour != null
    ? `${String(doc.memory_hour).padStart(2, '0')}:${String(doc.memory_minute ?? 0).padStart(2, '0')}`
    : ''
  return {
    year: doc.assoc_year ?? null,
    month: doc.assoc_month ?? null,
    day: doc.assoc_day ?? null,
    momento,
    time,
    period: doc.memory_period || '',
  }
}
