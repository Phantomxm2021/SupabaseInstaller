import { queryOptions } from '@tanstack/react-query'
import { apiFetch, setCSRFToken } from './client'

export interface SessionResponse {
  username: string
  mustChangePassword: boolean
  csrfToken: string
}

/** The sole session query definition; every observer shares CSRF synchronization. */
export function sessionQueryOptions() {
  return queryOptions({
    queryKey: ['session'] as const,
    queryFn: async () => {
      const current = await apiFetch<SessionResponse>('/api/session')
      setCSRFToken(current.csrfToken)
      return current
    },
  })
}
