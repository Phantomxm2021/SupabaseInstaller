import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ProjectsPage } from './ProjectsPage'

it('shows project health and the new project action', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', domain: 'bee.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter><ProjectsPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(await screen.findByText('Bee')).toBeVisible()
  expect(screen.getByText('Healthy')).toBeVisible()
  expect(screen.getByRole('link', { name: 'New project' })).toHaveAttribute('href', '/projects/new')
  expect(screen.getByTestId('projects-card')).toHaveAttribute('data-slot', 'card')
  expect(screen.getByRole('table')).toHaveAttribute('data-slot', 'table')
  expect(screen.getByText('Healthy')).toHaveAttribute('data-slot', 'badge')
})

it('announces project query failures as alerts', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { message: 'Projects unavailable' } }), { status: 503 })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter><ProjectsPage /></MemoryRouter></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Projects unavailable')
})

it('hides the project table header while the project list is empty', async () => {
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
