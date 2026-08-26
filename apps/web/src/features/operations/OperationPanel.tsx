import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, LoaderCircle, RotateCcw, ShieldAlert } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useInRouterContext, useNavigate } from 'react-router-dom'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Progress, ProgressLabel, ProgressValue } from '@/components/ui/progress'
import { apiFetch } from '../../api/client'
import type { Operation } from '../../api/types'
import { useOperationEvents } from './useOperationEvents'
const labels: Record<string, string> = { VALIDATE_PORTS: 'Validate ports', GENERATE_SECRETS: 'Generate secrets', START_RUNTIME: 'Start runtime', START_AUTH: 'Start Auth', FINAL_HEALTH_CHECK: 'Final health check', MARK_RUNNING: 'Mark project running' }
type Props = { operationId: string; projectId?: string; projectName: string; onSucceeded?: (projectId: string) => void }
export function OperationPanel(props: Props) { const inRouter = useInRouterContext(); return inRouter ? <RoutedOperationPanel {...props} /> : <OperationPanelCore {...props} /> }
function RoutedOperationPanel(props: Props) { const navigate = useNavigate(); return <OperationPanelCore {...props} navigate={navigate} /> }
function OperationPanelCore({ operationId, projectId, projectName, onSucceeded, navigate }: Props & { navigate?: (to: string, options?: { replace?: boolean }) => void }) {
  const [activeOperationId, setActiveOperationId] = useState(operationId); const handledSuccess = useRef<string | undefined>(undefined); const queryClient = useQueryClient(); const operation = useQuery({ queryKey: ['operation', activeOperationId], queryFn: () => apiFetch<Operation>(`/api/operations/${activeOperationId}`), refetchInterval: (query) => terminal(query.state.data?.status) ? false : 2_000 }); useOperationEvents(activeOperationId)
  const action = useMutation({ mutationFn: (name: 'retry' | 'rollback') => apiFetch<{ operationId: string }>(`/api/projects/${operation.data?.projectId}/${name}`, { method: 'POST' }), onSuccess: (result) => { setActiveOperationId(result.operationId); void queryClient.invalidateQueries({ queryKey: ['operation', result.operationId] }) } }); const current = operation.data
  useEffect(() => { const id = current?.projectId || projectId; if (current?.status !== 'SUCCEEDED' || !id || handledSuccess.current === activeOperationId) return; handledSuccess.current = activeOperationId; void (async () => { await queryClient.invalidateQueries({ queryKey: ['projects'] }); if (onSucceeded) onSucceeded(id); else navigate?.(`/projects/${id}/overview`, { replace: true }) })() }, [activeOperationId, current?.projectId, current?.status, navigate, onSucceeded, projectId, queryClient])
  const failed = current?.status === 'FAILED'; const rolledBack = current?.status === 'ROLLED_BACK'; const succeeded = current?.status === 'SUCCEEDED'; const step = labels[current?.currentStep ?? ''] ?? current?.currentStep ?? 'Waiting for worker'
  return <Card className="operation-card" aria-live="polite"><div className="operation-heading"><span className={`operation-icon ${failed ? 'failed' : succeeded ? 'done' : ''}`}>{failed ? <AlertTriangle /> : succeeded ? <Check /> : <LoaderCircle className="spin" />}</span><div><p className="eyebrow">Operation {activeOperationId}</p><h2>{succeeded ? `${projectName} is ready` : failed ? 'Installation needs attention' : rolledBack ? 'Installation rolled back safely' : `Installing ${projectName}`}</h2></div><Badge variant={failed ? 'destructive' : succeeded ? 'default' : 'outline'}>{current?.status ?? 'QUEUED'}</Badge></div><Progress value={current?.progress ?? 0} aria-label="Operation progress" className="my-6"><ProgressLabel>Progress</ProgressLabel><ProgressValue /></Progress><div className="current-step" role="status"><small>Current step</small><strong>{step}</strong></div>{current?.errorMessage && <Alert variant="destructive"><ShieldAlert className="size-4" /><span>{current.errorMessage}</span></Alert>}{(failed || rolledBack) && <div className="operation-actions"><Button variant="secondary" disabled={action.isPending} onClick={() => action.mutate('retry')}><RotateCcw className="size-4" />Retry</Button>{failed && <Button variant="destructive" disabled={action.isPending} onClick={() => action.mutate('rollback')}>Rollback</Button>}</div>}</Card>
}
function terminal(status?: Operation['status']) { return status ? ['SUCCEEDED', 'FAILED', 'ROLLED_BACK', 'CANCELLED'].includes(status) : false }
