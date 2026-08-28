import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { OverviewPage } from './OverviewPage'

it('opens the configured domain in Supabase Studio when Studio is healthy', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
    id: 'bee', name: 'Bee', slug: 'bee', domain: 'studio.example.com', siteUrl: 'https://app.example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0', preset: 'LIGHTWEIGHT',
    services: { database: true, gateway: true, auth: true, rest: true, studio: true, postgresMeta: true },
  }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/overview']}><Routes><Route path="/projects/:projectId/overview" element={<OverviewPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  const studio = await screen.findByRole('link', { name: 'Open Supabase Studio' })
  expect(studio).toHaveAttribute('href', 'https://studio.example.com')
  expect(studio).toHaveAttribute('target', '_blank')
})

it('uses the Supabase-style overview hierarchy while keeping local project data', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
    id: 'bee', name: 'Bee', slug: 'bee', domain: 'bee.example.com', siteUrl: 'https://example.com', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'self-hosted/v0.8.0', preset: 'LIGHTWEIGHT', configurationRevision: 4,
    services: { database: true, gateway: true, auth: true, rest: true, studio: true, postgresMeta: true, realtime: false, storage: false, imgproxy: false, functions: false, supavisor: false, logs: false, vector: false, directDb: false },
  }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/overview']}><Routes><Route path="/projects/:projectId/overview" element={<OverviewPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  const hero = await screen.findByTestId('project-overview-hero')
  expect(hero).toBeVisible()
  expect(within(hero).getByText('Status')).toBeVisible()
  expect(within(hero).getByText('Compute')).toBeVisible()
  expect(within(hero).getByText('Primary Database')).toBeVisible()
  expect(within(hero).getByText('6 active services')).toBeVisible()
  expect(screen.getByTestId('overview-services-card')).toBeVisible()
})

it('starts a stopped project through a durable operation', async () => {
  let mutationPath = ''
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (init?.method === 'POST') {
      mutationPath = path
      return new Response(JSON.stringify({ projectId: 'bee', operationId: 'operation-2' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
    }
    return new Response(JSON.stringify({ id: 'bee', name: 'Bee', slug: 'bee', domain: 'bee.example.com', siteUrl: 'https://example.com', status: 'STOPPED', health: 'STOPPED', supabaseVersion: 'self-hosted/v0.8.0', preset: 'LIGHTWEIGHT', services: { database: true, gateway: true, auth: true, rest: true, studio: true, postgresMeta: true } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/overview']}><Routes><Route path="/projects/:projectId/overview" element={<OverviewPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  await user.click(await screen.findByRole('button', { name: 'Start project' }))

  expect(mutationPath).toBe('/api/projects/bee/start')
  expect(await screen.findByText('Starting project')).toBeVisible()
  expect(screen.getByTestId('overview-services-card')).toHaveAttribute('data-slot', 'card')
  expect(screen.getByRole('table')).toHaveAttribute('data-slot', 'table')
})

it('retries a failed project from the project homepage', async () => {
  let mutationPath = ''
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (init?.method === 'POST') {
      mutationPath = path
      return new Response(JSON.stringify({ projectId: 'bee', operationId: 'retry-3' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
    }
    return new Response(JSON.stringify({ id: 'bee', name: 'Bee', slug: 'bee', domain: 'bee.example.com', siteUrl: 'https://example.com', status: 'FAILED', health: 'UNKNOWN', supabaseVersion: 'self-hosted/v0.8.0', preset: 'LIGHTWEIGHT', services: { database: true, gateway: true, auth: true, rest: true, studio: true, postgresMeta: true } }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/overview']}><Routes><Route path="/projects/:projectId/overview" element={<OverviewPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  await user.click(await screen.findByRole('button', { name: 'Retry project' }))

  await waitFor(() => expect(mutationPath).toBe('/api/projects/bee/retry'))
  expect(await screen.findByText('Retry queued')).toBeVisible()
})
