let csrfToken = ''

import type { AuthKeysOperationRequest, AuthKeysOperationResponse, SecretRevealResponse } from './types'

export class APIError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly fields?: Record<string, string>,
  ) {
    super(message)
  }
}

export function setCSRFToken(token: string) {
  csrfToken = token
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  if (csrfToken && init.method && !['GET', 'HEAD'].includes(init.method)) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(path, { ...init, headers, credentials: 'include' })
  if (!response.ok) {
    const payload = await response.json().catch(() => null) as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | null
    throw new APIError(response.status, payload?.error?.code ?? 'REQUEST_FAILED', payload?.error?.message ?? 'The request failed', payload?.error?.fields)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function revealSecret(projectId: string, kind: 'anonKey' | 'serviceRoleKey' | 'jwtSecret' | 'databasePassword' | 'publishable-api-key' | 'secret-api-key', password: string) {
  return apiFetch<SecretRevealResponse>(`/api/projects/${projectId}/secrets/${kind}/reveal`, {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function migrateAuthKeys(projectId: string, input: AuthKeysOperationRequest) {
  return apiFetch<AuthKeysOperationResponse>(`/api/projects/${projectId}/auth-keys/migrate`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function rotateApiKeys(projectId: string, input: AuthKeysOperationRequest) {
  return apiFetch<AuthKeysOperationResponse>(`/api/projects/${projectId}/auth-keys/rotate-api`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function rotateSigningKeys(projectId: string, input: AuthKeysOperationRequest) {
  return apiFetch<AuthKeysOperationResponse>(`/api/projects/${projectId}/auth-keys/rotate-signing`, {
    method: 'POST',
    body: JSON.stringify(input),
  })
}
