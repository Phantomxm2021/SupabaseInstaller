import { refreshProjectQueriesAfterDelete } from './LifecycleActions'

it('refreshes deletion queries in safe order before navigation', async () => {
  const calls: string[] = []
  const queryClient = {
    cancelQueries: vi.fn(async ({ queryKey }: { queryKey: string[] }) => { calls.push(`cancel:${queryKey.join('/')}`) }),
    removeQueries: vi.fn(({ queryKey }: { queryKey: string[] }) => { calls.push(`remove:${queryKey.join('/')}`) }),
    invalidateQueries: vi.fn(async ({ queryKey }: { queryKey: string[] }) => { calls.push(`invalidate:${queryKey.join('/')}`) }),
  }

  await refreshProjectQueriesAfterDelete(queryClient as never, 'bee')

  expect(calls).toEqual([
    'cancel:project/bee',
    'remove:project/bee',
    'remove:project-configuration/bee',
    'invalidate:projects',
  ])
})
