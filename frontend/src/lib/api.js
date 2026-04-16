/**
 * api.js — Authenticated fetch wrapper for PKD API.
 *
 * Reads the CSRF token from the `pkd_csrf` cookie and attaches it as
 * X-CSRF-Token on every mutating request (POST, PUT, DELETE, PATCH).
 * The session cookie is sent automatically by the browser (same-origin).
 */

/** Read a cookie value by name. Returns '' if not found. */
function getCookie(name) {
  const match = document.cookie
    .split(';')
    .map(c => c.trim())
    .find(c => c.startsWith(name + '='))
  return match ? decodeURIComponent(match.slice(name.length + 1)) : ''
}

const MUTATING = new Set(['POST', 'PUT', 'DELETE', 'PATCH'])

/**
 * apiFetch — wraps fetch with CSRF token and JSON defaults.
 *
 * @param {string} url - Relative URL (e.g. '/api/documents')
 * @param {RequestInit} [options]
 * @returns {Promise<Response>}
 */
export async function apiFetch(url, options = {}) {
  const method = (options.method || 'GET').toUpperCase()
  const headers = new Headers(options.headers)

  if (MUTATING.has(method)) {
    const csrf = getCookie('pkd_csrf')
    if (csrf) headers.set('X-CSRF-Token', csrf)
  }

  // Default Content-Type for JSON bodies
  if (options.body && typeof options.body === 'string' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  return fetch(url, { ...options, method, headers })
}

/** Convenience: GET and parse JSON. Throws on non-2xx. */
export async function apiGet(url) {
  const res = await apiFetch(url)
  if (!res.ok) throw new ApiError(res.status, await res.text())
  return res.json()
}

/** Convenience: POST JSON body and parse JSON response. Throws on non-2xx. */
export async function apiPost(url, body) {
  const res = await apiFetch(url, {
    method: 'POST',
    body: body != null ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new ApiError(res.status, await res.text())
  if (res.status === 204 || res.headers.get('Content-Length') === '0') return null
  return res.json()
}

/** Convenience: PUT JSON body and parse JSON response. */
export async function apiPut(url, body) {
  const res = await apiFetch(url, {
    method: 'PUT',
    body: body != null ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    if (res.status === 409) return { _conflict: true, ...(await res.json()) }
    throw new ApiError(res.status, await res.text())
  }
  if (res.status === 204) return null
  return res.json()
}

/** Convenience: DELETE request. Throws on non-2xx. */
export async function apiDelete(url) {
  const res = await apiFetch(url, { method: 'DELETE' })
  if (!res.ok) throw new ApiError(res.status, await res.text())
  return null
}

export class ApiError extends Error {
  constructor(status, message) {
    super(message || `HTTP ${status}`)
    this.status = status
  }
}
