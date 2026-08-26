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
