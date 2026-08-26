import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { Pause, Play, RotateCw, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '../../api/client'
import type { Project } from '../../api/types'
import { toast } from 'sonner'
import { DeleteProjectDialog } from './DeleteProjectDialog'

type LifecycleAction = 'start' | 'stop' | 'restart'

export async function refreshProjectQueriesAfterDelete(queryClient: Pick<QueryClient, 'cancelQueries' | 'removeQueries' | 'invalidateQueries'>, projectId: string) {
  await queryClient.cancelQueries({ queryKey: ['project', projectId] })
  await queryClient.cancelQueries({ queryKey: ['project-configuration', projectId] })
  queryClient.removeQueries({ queryKey: ['project', projectId] })
  queryClient.removeQueries({ queryKey: ['project-configuration', projectId] })
  await queryClient.invalidateQueries({ queryKey: ['projects'] })
}

const actionLabels: Record<LifecycleAction, string> = {
  start: 'Starting project',
  stop: 'Stopping project',
  restart: 'Restarting project',
}

export function LifecycleActions({ project }: { project: Project }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [message, setMessage] = useState('')
  const [deleteOpen, setDeleteOpen] = useState(false)
  const lifecycle = useMutation({
    mutationFn: (action: LifecycleAction) => apiFetch<{ projectId: string; operationId: string }>(`/api/projects/${project.id}/${action}`, { method: 'POST' }),
    onMutate: (action) => setMessage(actionLabels[action]),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['project', project.id] }),
  })
  const remove = useMutation({
    mutationFn: ({ mode, confirmation }: { mode: 'runtime' | 'data'; confirmation: string }) => apiFetch(`/api/projects/${project.id}`, { method: 'DELETE', body: JSON.stringify({ mode, confirmation }) }),
    onSuccess: async () => {
      await refreshProjectQueriesAfterDelete(queryClient, project.id)
      toast.success('Project deleted')
      navigate('/projects', { replace: true })
    },
    onError: (error) => toast.error(error.message),
  })

  return (
    <div className="lifecycle-wrap">
      <div className="lifecycle-actions">
        {project.status === 'STOPPED' && <button className="button primary" disabled={lifecycle.isPending} onClick={() => lifecycle.mutate('start')}><Play size={15} /> Start project</button>}
        {['RUNNING', 'DEGRADED'].includes(project.status) && <>
          <button className="button secondary" disabled={lifecycle.isPending} onClick={() => lifecycle.mutate('stop')}><Pause size={15} /> Stop</button>
          <button className="button secondary" disabled={lifecycle.isPending} onClick={() => lifecycle.mutate('restart')}><RotateCw size={15} /> Restart</button>
        </>}
        <button className="button danger" type="button" onClick={() => setDeleteOpen(true)}><Trash2 size={15} /> Delete</button>
      </div>
      {message && <span className="action-status" role="status">{message}</span>}
      {(lifecycle.error || remove.error) && <div className="alert error">{(lifecycle.error ?? remove.error)?.message}</div>}
      <DeleteProjectDialog project={project} open={deleteOpen} busy={remove.isPending} onClose={() => setDeleteOpen(false)} onDelete={(mode, confirmation) => remove.mutate({ mode, confirmation })} />
    </div>
  )
}
