import { writable } from 'svelte/store'
import { apiFetch } from '../api.js'

/** Whether the user is currently authenticated. null = unknown (checking). */
export const authenticated = writable(null)

/**
 * Attempt login with the master password.
 * @param {string} password
 * @returns {Promise<{ok: boolean, status?: number, twoFactor?: boolean, challengeId?: string}>}
 */
export async function login(password) {
  const res = await apiFetch('/api/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
  if (res.status === 204) {
    authenticated.set(true)
    return { ok: true, twoFactor: false }
  }
  if (res.status === 200) {
    const body = await res.json()
    return { ok: true, twoFactor: true, challengeId: body.challenge_id }
  }
  return { ok: false, status: res.status }
}

/**
 * Complete a login 2FA challenge with the e-mailed code.
 * @param {string} challengeId
 * @param {string} code
 * @returns {Promise<{ok: boolean, status: number}>}
 */
export async function submit2FA(challengeId, code) {
  const res = await apiFetch('/api/login/2fa', {
    method: 'POST',
    body: JSON.stringify({ challenge_id: challengeId, code }),
  })
  if (res.ok) authenticated.set(true)
  return { ok: res.ok, status: res.status }
}

/** Log out and redirect to login page. */
export async function logout() {
  await apiFetch('/api/logout', { method: 'POST' })
  authenticated.set(false)
}

/**
 * Check if a session exists by hitting a lightweight authenticated endpoint.
 * Sets the authenticated store accordingly.
 */
export async function checkSession() {
  try {
    const res = await apiFetch('/api/tags')
    authenticated.set(res.ok)
  } catch {
    authenticated.set(false)
  }
}
