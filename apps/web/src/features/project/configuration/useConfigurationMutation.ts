import { useMutation } from '@tanstack/react-query'
import { useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '../../../api/client'
import type { UpdateSecretInput } from '../../../api/types'
import { sectionEndpoint, type ConfigurationSection, type PendingConfigurationSave } from './types'

export type ConfigurationOperation = { projectId: string; operationId: string; revision: number }

function normalizeSecrets(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalizeSecrets)
  if (!value || typeof value !== 'object') return value
  const output: Record<string, unknown> = {}
  for (const [key, child] of Object.entries(value)) {
    if (key === 'value' && typeof (value as { action?: unknown }).action === 'string' && (value as { action: string }).action !== 'replace') continue
    if (key === 'action' && child === '') { output.action = ''; continue }
    output[key] = normalizeSecrets(child)
  }
  // Redacted responses carry a configured marker but intentionally omit the
  // command. At the update boundary that marker means retain unless the user
  // chose replace/remove in the section form.
  for (const [setKey, secretKey] of [['passwordSet', 'password'], ['secretSet', 'secret'], ['secretAccessKeySet', 'secretAccessKey'], ['valueSet', 'value']] as const) {
    const secret = output[secretKey]
    if (output[setKey] === true && secret && typeof secret === 'object' && (secret as { action?: unknown }).action === '') output[secretKey] = { action: 'retain' }
  }
  return output
}

export function normalizeConfigurationValue(value: unknown) { return normalizeSecrets(structuredClone(value)) }

export function useConfigurationMutation(projectId: string, revision: number, onQueued: (result: ConfigurationOperation) => void, onError?: (error: Error) => void) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (pending: PendingConfigurationSave) => {
      const endpoint = sectionEndpoint(pending.section, pending.provider)
      return apiFetch<ConfigurationOperation>(`/api/projects/${projectId}/configuration/${endpoint}`, { method: 'PATCH', body: JSON.stringify({ expectedRevision: revision, value: normalizeConfigurationValue(pending.value) }) })
    },
    onSuccess: (result) => {
      onQueued(result)
      void queryClient.invalidateQueries({ queryKey: ['project', projectId] })
    },
    onError: (error) => onError?.(error instanceof Error ? error : new Error('Configuration update failed')),
  })
}

export type { UpdateSecretInput }
