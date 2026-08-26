import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'

const eventNames = ['OPERATION_QUEUED', 'OPERATION_STARTED', 'STEP_STARTED', 'STEP_COMPLETED', 'OPERATION_FAILED', 'ROLLBACK_STARTED', 'ROLLBACK_COMPLETED', 'OPERATION_SUCCEEDED']

export function useOperationEvents(operationId: string) {
  const queryClient = useQueryClient()
  useEffect(() => {
    if (!operationId || typeof EventSource === 'undefined') return
    const source = new EventSource(`/api/operations/${operationId}/events`, { withCredentials: true })
    const refresh = () => void queryClient.invalidateQueries({ queryKey: ['operation', operationId] })
    eventNames.forEach((name) => source.addEventListener(name, refresh))
    source.onerror = refresh
    return () => source.close()
  }, [operationId, queryClient])
}
