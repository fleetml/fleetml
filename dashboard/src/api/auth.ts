// Dashboard session. Logging in exchanges the admin API key for an httpOnly
// session cookie, so the key is never stored in the browser. Requests that
// change state send the session's CSRF token, which lives only in memory and
// is fetched again from GET /auth/session after a reload.

let csrfToken = ''

export function getCsrfToken(): string {
  return csrfToken
}

export function setCsrfToken(token: string): void {
  csrfToken = token
}

/** Fired on window when the API answers 401: the session expired or was revoked. */
export const AUTH_EVENT = 'fleetml:unauthorized'

export class UnauthorizedError extends Error {
  constructor(message = 'Your session expired or was revoked') {
    super(message)
    this.name = 'UnauthorizedError'
  }
}

export function notifyUnauthorized(message: string): void {
  window.dispatchEvent(new CustomEvent(AUTH_EVENT, { detail: message }))
}
