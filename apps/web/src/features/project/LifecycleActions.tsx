import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { ChevronDown, Pause, Play, RotateCw, Trash2 } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '../../api/client'
import type { Project } from '../../api/types'
import { toast } from 'sonner'
import { DeleteProjectDialog } from './DeleteProjectDialog'
import { Button } from '@/components/ui/button'
import { Alert } from '@/components/ui/alert'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

type LifecycleAction = 'start' | 'stop' | 'restart'

export async function refreshProjectQueriesAfterDelete(queryClient: Pick<QueryClient, 'cancelQueries' | 'removeQueries' | 'invalidateQueries'>, projectId: string) {
  await queryClient.cancelQueries({ queryKey: ['project', projectId] })
  await queryClient.cancelQueries({ queryKey: ['project-configuration', projectId] })
  queryClient.removeQueries({ queryKey: ['project', projectId] })
  queryClient.removeQueries({ queryKey: ['project-configuration', projectId] })
  await queryClient.invalidateQueries({ queryKey: ['projects'] })
}

const actionLabels: Record<LifecycleAction, string> = {
  start: 'Starting Project',
  stop: 'Stopping Project',
  restart: 'Restarting Project',
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
  const retry = useMutation({
    mutationFn: () => apiFetch<{ projectId: string; operationId: string }>(`/api/projects/${project.id}/retry`, { method: 'POST' }),
    onMutate: () => setMessage('Retrying Project…'),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['project', project.id] })
      await queryClient.invalidateQueries({ queryKey: ['projects'] })
      setMessage('Project retry queued')
    },
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
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button type="button" variant="secondary" aria-label="Actions" />}>
            Actions <ChevronDown className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" sideOffset={8}>
            {project.status === 'FAILED' && <DropdownMenuItem disabled={retry.isPending} onClick={() => retry.mutate()}><RotateCw /> {retry.isPending ? 'Retrying Project…' : 'Retry Project'}</DropdownMenuItem>}
            {project.status === 'STOPPED' && <DropdownMenuItem disabled={lifecycle.isPending} onClick={() => lifecycle.mutate('start')}><Play /> {lifecycle.isPending ? 'Starting Project…' : 'Start Project'}</DropdownMenuItem>}
            {['RUNNING', 'DEGRADED'].includes(project.status) && <>
              <DropdownMenuItem disabled={lifecycle.isPending} onClick={() => lifecycle.mutate('stop')}><Pause /> {lifecycle.isPending ? 'Stopping Project…' : 'Stop Project'}</DropdownMenuItem>
              <DropdownMenuItem disabled={lifecycle.isPending} onClick={() => lifecycle.mutate('restart')}><RotateCw /> {lifecycle.isPending ? 'Restarting Project…' : 'Restart Project'}</DropdownMenuItem>
            </>}
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={() => setDeleteOpen(true)}><Trash2 /> Delete Project</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {message && <span className="action-status" role="status">{message}</span>}
      {(lifecycle.error || retry.error || remove.error) && <Alert variant="destructive">{(lifecycle.error ?? retry.error ?? remove.error)?.message}</Alert>}
      <DeleteProjectDialog project={project} open={deleteOpen} busy={remove.isPending} onClose={() => setDeleteOpen(false)} onDelete={(mode, confirmation) => remove.mutate({ mode, confirmation })} />
    </div>
  )
}
