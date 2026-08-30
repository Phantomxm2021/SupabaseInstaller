import { refreshProjectQueriesAfterDelete } from './LifecycleActions'
import { LifecycleActions } from './LifecycleActions'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { useEffect } from 'react'

const timeline = vi.hoisted(() => [] as string[])
vi.mock('sonner', () => ({ toast: { success: vi.fn(() => timeline.push('toast:success')), error: vi.fn(() => timeline.push('toast:error')) } }))

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
  vi.spyOn(queryClient, 'cancelQueries').mockImplementation(async (filters) => { calls.push(`cancel:${(filters?.queryKey ?? []).join('/')}`); timeline.push(`cancel:${(filters?.queryKey ?? []).join('/')}`) })
  vi.spyOn(queryClient, 'removeQueries').mockImplementation((filters) => { calls.push(`remove:${(filters?.queryKey ?? []).join('/')}`); timeline.push(`remove:${(filters?.queryKey ?? []).join('/')}`) })
  vi.spyOn(queryClient, 'invalidateQueries').mockImplementation(async (filters) => { calls.push(`invalidate:${(filters?.queryKey ?? []).join('/')}`); timeline.push(`invalidate:${(filters?.queryKey ?? []).join('/')}`); return undefined })
  const requests: Array<{ input: RequestInfo | URL; init?: RequestInit }> = []
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => { calls.push('api'); requests.push({ input, init }); return new Response(null, { status: 204 }) }))
  const project = { id: 'bee', name: 'Bee', status: 'RUNNING', health: 'HEALTHY', services: {} } as never
  function Location() { const location = useLocation(); const navigate = useNavigate(); useEffect(() => { timeline.push(`navigate:${location.pathname}`) }, [location.pathname]); return <><output data-testid="location">{location.pathname}</output><button onClick={() => navigate(-1)}>Back</button></> }
  render(<QueryClientProvider client={queryClient}><MemoryRouter initialEntries={['/projects/old', '/projects/bee']} initialIndex={1}><LifecycleActions project={project} /><Location /></MemoryRouter></QueryClientProvider>)
  timeline.length = 0
  await user.click(screen.getByRole('button', { name: 'Actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete Project' }))
  await user.click(screen.getByRole('button', { name: /delete permanently/i }))
  expect(await screen.findByTestId('location')).toHaveTextContent('/projects')
  expect(requests[0].input).toBe('/api/projects/bee')
  expect(requests[0].init?.method).toBe('DELETE')
  expect(JSON.parse(String(requests[0].init?.body))).toEqual({ mode: 'runtime', confirmation: '' })
  expect(calls).toEqual(['api', 'cancel:project/bee', 'cancel:project-configuration/bee', 'remove:project/bee', 'remove:project-configuration/bee', 'invalidate:projects'])
  expect(timeline).toEqual(['cancel:project/bee', 'cancel:project-configuration/bee', 'remove:project/bee', 'remove:project-configuration/bee', 'invalidate:projects', 'toast:success', 'navigate:/projects'])
  await user.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByTestId('location')).toHaveTextContent('/projects/old')
})

it('shows an error toast and stays on the project when deletion fails', async () => {
  timeline.length = 0
  const queryClient = new QueryClient()
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { message: 'delete failed' } }), { status: 500, headers: { 'Content-Type': 'application/json' } })))
  const project = { id: 'bee', name: 'Bee', status: 'RUNNING', health: 'HEALTHY', services: {} } as never
  function CurrentLocation() { return <output data-testid="location">{useLocation().pathname}</output> }
  render(<QueryClientProvider client={queryClient}><MemoryRouter initialEntries={['/projects/bee']}><LifecycleActions project={project} /><CurrentLocation /></MemoryRouter></QueryClientProvider>)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete Project' }))
  await user.click(screen.getByRole('button', { name: /delete permanently/i }))
  await waitFor(() => expect(screen.getByText('delete failed')).toBeInTheDocument())
  expect(timeline).toEqual(['toast:error'])
  expect(screen.getByTestId('location')).toHaveTextContent('/projects/bee')
})

it('offers stopped-server lifecycle and deletion actions in the menu', async () => {
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
  const project = { id: 'bee', name: 'Bee', status: 'STOPPED', health: 'STOPPED', services: {} } as never
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><LifecycleActions project={project} /></MemoryRouter></QueryClientProvider>)
  const user = userEvent.setup()
  const actions = screen.getByRole('button', { name: 'Actions' })
  expect(actions).toHaveAttribute('aria-haspopup', 'menu')
  await user.click(actions)
  const start = await screen.findByRole('menuitem', { name: 'Start Project' })
  expect(screen.getByRole('menuitem', { name: 'Delete Project' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Stop Project' })).not.toBeInTheDocument()
  await user.click(start)
  await user.click(actions)
  const pendingStart = await screen.findByRole('menuitem', { name: 'Starting Project…' })
  expect(pendingStart).toHaveAttribute('aria-disabled', 'true')
})

it.each([
  ['FAILED', 'Retry Project'],
  ['RUNNING', 'Stop Project'],
  ['DEGRADED', 'Restart Project'],
] as const)('shows the %s server action in the Actions menu', async (status, action) => {
  const project = { id: 'bee', name: 'Bee', status, health: status, services: {} } as never
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><LifecycleActions project={project} /></MemoryRouter></QueryClientProvider>)

  await userEvent.setup().click(screen.getByRole('button', { name: 'Actions' }))

  expect(await screen.findByRole('menuitem', { name: action })).toBeInTheDocument()
  expect(screen.getByRole('separator')).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Delete Project' })).toBeInTheDocument()
})
