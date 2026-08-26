import { refreshProjectQueriesAfterDelete } from './LifecycleActions'
import { LifecycleActions } from './LifecycleActions'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

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
    'cancel:project-configuration/bee',
    'remove:project/bee',
    'remove:project-configuration/bee',
    'invalidate:projects',
  ])
})

it('deletes through the API, refreshes caches, toasts, and replaces route', async () => {
  const user = userEvent.setup()
  const calls: string[] = []
  const queryClient = new QueryClient()
  vi.spyOn(queryClient, 'cancelQueries').mockImplementation(async (filters) => { calls.push(`cancel:${(filters?.queryKey ?? []).join('/')}`) })
  vi.spyOn(queryClient, 'removeQueries').mockImplementation((filters) => { calls.push(`remove:${(filters?.queryKey ?? []).join('/')}`) })
  vi.spyOn(queryClient, 'invalidateQueries').mockImplementation(async (filters) => { calls.push(`invalidate:${(filters?.queryKey ?? []).join('/')}`); return undefined })
  vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })))
  const project = { id: 'bee', name: 'Bee', status: 'RUNNING', health: 'HEALTHY', services: {} } as never
  function Location() { const location = useLocation(); return <output data-testid="location">{location.pathname}</output> }
  render(<QueryClientProvider client={queryClient}><MemoryRouter><LifecycleActions project={project} /><Location /></MemoryRouter></QueryClientProvider>)
  await user.click(screen.getByRole('button', { name: /delete/i }))
  await user.click(screen.getByRole('button', { name: /delete permanently/i }))
  expect(await screen.findByTestId('location')).toHaveTextContent('/projects')
  expect(calls).toEqual(['cancel:project/bee', 'cancel:project-configuration/bee', 'remove:project/bee', 'remove:project-configuration/bee', 'invalidate:projects'])
})
