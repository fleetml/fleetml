// Admin API key storage for the dashboard. The key lives in this browser's
// localStorage and is sent as "Authorization: Bearer <key>" on every request.

const STORAGE_KEY = 'fleetml.apiKey'

/** Fired on window whenever the API rejects the key (or none is set). */
export const AUTH_EVENT = 'fleetml:unauthorized'

export class UnauthorizedError extends Error {
  constructor(message = 'API key missing or rejected') {
    super(message)
    this.name = 'UnauthorizedError'
  }
}

export function getApiKey(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) || ''
  } catch {
    return ''
  }
}

export function setApiKey(key: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, key.trim())
  } catch {
    // Storage unavailable (private mode, blocked site data): nothing to persist.
  }
}

export function clearApiKey(): void {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
}

export function notifyUnauthorized(message: string): void {
  window.dispatchEvent(new CustomEvent(AUTH_EVENT, { detail: message }))
}
