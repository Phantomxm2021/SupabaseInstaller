import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { OverviewPage } from './OverviewPage'

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
})
