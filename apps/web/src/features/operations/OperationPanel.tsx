import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, LoaderCircle, RotateCcw, ShieldAlert } from 'lucide-react'
import { useState } from 'react'
import { apiFetch } from '../../api/client'
import type { Operation } from '../../api/types'
import { useOperationEvents } from './useOperationEvents'

const labels: Record<string, string> = {
  VALIDATE_PORTS: 'Validate ports',
  GENERATE_SECRETS: 'Generate secrets',
  PREPARE_SUPABASE: 'Prepare Supabase',
  START_RUNTIME: 'Start runtime',
  START_AUTH: 'Start Auth',
  FINAL_HEALTH_CHECK: 'Final health check',
  MARK_RUNNING: 'Mark project running',
}

export function OperationPanel({ operationId, projectName }: { operationId: string; projectName: string }) {
	const [activeOperationId, setActiveOperationId] = useState(operationId)
  const queryClient = useQueryClient()
  const operation = useQuery({
    queryKey: ['operation', activeOperationId],
    queryFn: () => apiFetch<Operation>(`/api/operations/${activeOperationId}`),
    refetchInterval: (query) => terminal(query.state.data?.status) ? false : 2_000,
  })
  useOperationEvents(activeOperationId)
  const action = useMutation({
    mutationFn: (name: 'retry' | 'rollback') => apiFetch<{ operationId: string }>(`/api/projects/${operation.data?.projectId}/${name}`, { method: 'POST' }),
    onSuccess: (result) => {
      setActiveOperationId(result.operationId)
      queryClient.invalidateQueries({ queryKey: ['operation', result.operationId] })
    },
  })
  const current = operation.data
  const failed = current?.status === 'FAILED'
  const rolledBack = current?.status === 'ROLLED_BACK'
  const succeeded = current?.status === 'SUCCEEDED'
  return (
    <section className="operation-card panel" aria-live="polite">
      <div className="operation-heading">
        <span className={`operation-icon ${failed ? 'failed' : succeeded ? 'done' : ''}`}>{failed ? <AlertTriangle /> : succeeded ? <Check /> : <LoaderCircle className="spin" />}</span>
        <div><p className="eyebrow">Operation {activeOperationId}</p><h2>{succeeded ? `${projectName} is ready` : failed ? `Installation needs attention` : rolledBack ? 'Installation rolled back safely' : `Installing ${projectName}`}</h2></div>
        <span className={`badge ${current?.status?.toLowerCase() ?? 'neutral'}`}>{current?.status ?? 'QUEUED'}</span>
      </div>
      <div className="progress-track"><span style={{ width: `${current?.progress ?? 0}%` }} /></div>
      <div className="current-step"><small>Current step</small><strong>{labels[current?.currentStep ?? ''] ?? current?.currentStep ?? 'Waiting for worker'}</strong></div>
      {current?.errorMessage && <div className="alert error"><ShieldAlert size={16} /><span>{current.errorMessage}</span></div>}
      {(failed || rolledBack) && <div className="operation-actions"><button className="button secondary" disabled={action.isPending} onClick={() => action.mutate('retry')}><RotateCcw size={15} />Retry</button>{failed && <button className="button danger" disabled={action.isPending} onClick={() => action.mutate('rollback')}>Rollback</button>}</div>}
    </section>
  )
}

function terminal(status?: Operation['status']) { return status ? ['SUCCEEDED', 'FAILED', 'ROLLED_BACK', 'CANCELLED'].includes(status) : false }
