import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ConfigurationPage } from './ConfigurationPage'
import { defaultConfiguration } from '../projects/projectSchema'

it('renders the installed project configuration workspace from the redacted snapshot', async () => {
  const configuration = defaultConfiguration('LIGHTWEIGHT')
  configuration.general = { domain: 'bee.example.com', siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' }
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ projectId: 'bee', revision: 4, lastGoodRevision: 4, configuration }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/configuration?section=database']}><Routes><Route path="/projects/:projectId/configuration" element={<ConfigurationPage />} /></Routes></MemoryRouter></QueryClientProvider>)
  expect(await screen.findByText('Database', { selector: '[data-slot="card-title"]' })).toBeVisible()
  expect(screen.getByLabelText('Maximum connections')).toBeVisible()
  expect(screen.getByRole('tab', { name: 'API & Secrets' })).toBeVisible()
})
