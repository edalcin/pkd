import { writable } from 'svelte/store'

const AUTOSAVE_KEY = 'pkd-autosave-interval'

function createAutoSaveInterval() {
  const stored = localStorage.getItem(AUTOSAVE_KEY)
  const initial = stored !== null ? Number(stored) : 5000
  const { subscribe, set: _set } = writable(initial)
  return {
    subscribe,
    set(val) {
      localStorage.setItem(AUTOSAVE_KEY, String(val))
      _set(val)
    },
  }
}

export const autoSaveInterval = createAutoSaveInterval()
