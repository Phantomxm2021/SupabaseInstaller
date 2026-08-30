import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProjectsPage } from './ProjectsPage'

function renderProjectsPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <MemoryRouter><ProjectsPage /></MemoryRouter>
    </QueryClientProvider>,
  )
}

it('shows server health and the new server action', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter><ProjectsPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(await screen.findByText('Bee')).toBeVisible()
  expect(screen.getByText('Healthy')).toBeVisible()
  expect(screen.getByRole('link', { name: 'New project' })).toHaveAttribute('href', '/projects/new')
  expect(screen.getByRole('link', { name: 'View Details' })).toHaveAttribute('href', '/projects/bee/overview')
  expect(screen.getByTestId('projects-card')).toHaveAttribute('data-slot', 'card')
  expect(screen.getByRole('table')).toHaveAttribute('data-slot', 'table')
  expect(screen.getByText('Healthy')).toHaveAttribute('data-slot', 'badge')
})

it('uses the shared page header and filters servers by name or domain', async () => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith('/api/host/resources')) return Promise.resolve(new Response(JSON.stringify({ cpuPercent: 0, cpuCores: 4, memoryUsedBytes: 1, memoryTotalBytes: 2, diskUsedBytes: 1, diskTotalBytes: 2 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    return Promise.resolve(new Response(JSON.stringify({ projects: [
      { id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0' },
      { id: 'atlas', name: 'Atlas', domain: 'atlas.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.9.0' },
    ] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  }))
  const user = userEvent.setup()
  renderProjectsPage()

  expect(await screen.findByRole('heading', { name: 'Projects' })).toBeVisible()
  expect(screen.getByText('Runtime orchestration')).toBeVisible()
  await user.type(screen.getByRole('textbox', { name: 'Search projects' }), 'atlas.example')

  expect(screen.getByText('Atlas')).toBeVisible()
  expect(screen.queryByText('Bee')).not.toBeInTheDocument()
})

it('shows resource skeletons while host resources are loading', async () => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith('/api/host/resources')) return new Promise<Response>(() => {})
    return Promise.resolve(new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  }))
  renderProjectsPage()

  expect(await screen.findByText('Bee')).toBeVisible()
  expect(screen.getAllByTestId('resource-skeleton')).toHaveLength(3)
})

it('shows the project-list loading AsyncState until the projects request resolves', async () => {
  let resolveProjects: (response: Response) => void
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith('/api/host/resources')) return Promise.resolve(new Response(JSON.stringify({ cpuPercent: 0, cpuCores: 4, memoryUsedBytes: 1, memoryTotalBytes: 2, diskUsedBytes: 1, diskTotalBytes: 2 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    return new Promise<Response>((resolve) => { resolveProjects = resolve })
  }))
  renderProjectsPage()

  const loadingState = screen.getByRole('status')
  expect(loadingState).toHaveTextContent('Loading')
  expect(loadingState.querySelector('[data-slot="skeleton"]')).toBeInTheDocument()

  resolveProjects!(new Response(JSON.stringify({ projects: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  expect(await screen.findByText('No projects yet')).toBeVisible()
})

it('retries a failed project-list query from its destructive alert', async () => {
  let projectRequests = 0
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith('/api/host/resources')) return Promise.resolve(new Response(JSON.stringify({ cpuPercent: 0, cpuCores: 4, memoryUsedBytes: 1, memoryTotalBytes: 2, diskUsedBytes: 1, diskTotalBytes: 2 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    projectRequests += 1
    if (projectRequests === 1) return Promise.resolve(new Response(JSON.stringify({ error: { message: 'Projects unavailable' } }), { status: 503 }))
    return Promise.resolve(new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  }))
  const user = userEvent.setup()
  renderProjectsPage()

  expect(await screen.findByRole('alert')).toHaveTextContent('Projects unavailable')
  await user.click(screen.getByRole('button', { name: 'Retry' }))

  expect(await screen.findByText('Bee')).toBeVisible()
  expect(projectRequests).toBe(2)
})

it('offers server creation from the empty state', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ projects: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  renderProjectsPage()

  expect(await screen.findByText('No projects yet')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Create project' })).toHaveAttribute('href', '/projects/new')
})

it('announces server query failures as alerts', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { message: 'Projects unavailable' } }), { status: 503 })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><ProjectsPage /></MemoryRouter></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Projects unavailable')
})

it('hides the server table header while the server list is empty', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ projects: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><ProjectsPage /></MemoryRouter></QueryClientProvider>)

  expect(await screen.findByText('No projects yet')).toBeVisible()
  expect(screen.queryByRole('columnheader', { name: 'Project' })).not.toBeInTheDocument()
})

it('renders host CPU, memory, and disk metrics from the resources endpoint', async () => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    const path = String(input)
    if (path.endsWith('/api/host/resources')) {
      return Promise.resolve(new Response(JSON.stringify({ cpuPercent: 31, cpuCores: 10, memoryUsedBytes: 6657199308, memoryTotalBytes: 16642998272, diskUsedBytes: 90194313216, diskTotalBytes: 214748364800 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    }
    return Promise.resolve(new Response(JSON.stringify({ projects: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  }))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><ProjectsPage /></MemoryRouter></QueryClientProvider>)

  expect(await screen.findByText('31%')).toBeVisible()
  expect(screen.getByText(/6\.2 GB \/ 15\.5 GB/)).toBeVisible()
  expect(screen.getByText(/84\.0 GB \/ 200\.0 GB/)).toBeVisible()
})

it('uses server terminology when disk metrics are unavailable', async () => {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith('/api/host/resources')) return Promise.resolve(new Response(JSON.stringify({ error: { message: 'Metrics unavailable' } }), { status: 503 }))
    return Promise.resolve(new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  }))
  renderProjectsPage()

  expect(await screen.findByText('Bee')).toBeVisible()
  expect(screen.getByText('Server data volume')).toBeVisible()
})

it('retries a failed project from the project list', async () => {
  const requests: Array<{ path: string; method?: string }> = []
  let resolveRetry: (response: Response) => void
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    requests.push({ path, method: init?.method })
    if (init?.method === 'POST') return new Promise<Response>((resolve) => { resolveRetry = resolve })
    if (path.endsWith('/api/host/resources')) return new Response(JSON.stringify({ cpuPercent: 0, cpuCores: 4, memoryUsedBytes: 1, memoryTotalBytes: 2, diskUsedBytes: 1, diskTotalBytes: 2 }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    return new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'FAILED', health: 'UNKNOWN', supabaseVersion: 'self-hosted/v0.8.0' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MemoryRouter><ProjectsPage /></MemoryRouter></QueryClientProvider>)

  await user.click(await screen.findByRole('button', { name: 'Retry Bee' }))

  await waitFor(() => expect(requests).toContainEqual({ path: '/api/projects/bee/retry', method: 'POST' }))
  expect(await screen.findByRole('status')).toHaveTextContent('Retrying project…')
  resolveRetry!(new Response(JSON.stringify({ projectId: 'bee', operationId: 'retry-1' }), { status: 202, headers: { 'Content-Type': 'application/json' } }))
  expect(await screen.findByRole('status')).toHaveTextContent('Retry queued')
})
