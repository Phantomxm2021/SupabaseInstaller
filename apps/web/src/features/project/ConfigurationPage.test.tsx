import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

function redactedSnapshot(domain = 'bee.example.com') {
  const configuration = defaultConfiguration('LIGHTWEIGHT')
  configuration.general = { domain, siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' }
  const redacted = JSON.parse(JSON.stringify(configuration)) as typeof configuration
  delete (redacted.auth as unknown as { redirectUrls?: string[] }).redirectUrls
  delete (redacted.auth as unknown as { oauth?: Record<string, unknown> }).oauth?.google
  delete (redacted.auth.phone as unknown as { provider?: string }).provider
  delete (redacted.auth.phone as unknown as { fields?: Record<string, string> }).fields
  delete (redacted.functions as unknown as { variables?: unknown[] }).variables
  delete (redacted.database as unknown as { extensions?: string[] }).extensions
  return { projectId: 'bee', revision: 4, lastGoodRevision: 4, configuration: redacted }
}

function renderConfiguration(section = 'general') {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={[`/projects/bee/configuration?section=${section}`]}><Routes><Route path="/projects/:projectId/configuration" element={<ConfigurationPage />} /></Routes></MemoryRouter></QueryClientProvider>)
}

it('keeps dirty input when preview is dismissed with Keep editing', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-1', revision: 5 }), { status: 202 })
    return new Response(JSON.stringify(redactedSnapshot()), { status: 200 })
  }))
  renderConfiguration()
  const domain = await screen.findByLabelText('Domain')
  await user.clear(domain)
  await user.type(domain, 'edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Save General' }))
  expect(await screen.findByRole('alertdialog')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Keep editing' }))
  expect(screen.getByLabelText('Domain')).toHaveValue('edited.example.com')
})

it('preserves dirty fields on 409 and only Reload resets to new server data', async () => {
  const user = userEvent.setup()
  let getCount = 0
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ error: { code: 'CONFIGURATION_STALE', message: 'stale' } }), { status: 409 })
    getCount += 1
    return new Response(JSON.stringify(redactedSnapshot(getCount > 1 ? 'server.example.com' : 'bee.example.com')), { status: 200 })
  }))
  renderConfiguration()
  const domain = await screen.findByLabelText('Domain')
  await user.clear(domain)
  await user.type(domain, 'edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Save General' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  expect(await screen.findByText('This configuration is stale. Your dirty fields are preserved.')).toBeVisible()
  expect(screen.getByLabelText('Domain')).toHaveValue('edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Reload' }))
  await waitFor(() => expect(screen.getByLabelText('Domain')).toHaveValue('server.example.com'))
})

it('renders authoritative API field errors', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ error: { code: 'INVALID_CONFIGURATION', message: 'invalid', fields: { domain: 'Domain is already used' } } }), { status: 422 })
    return new Response(JSON.stringify(redactedSnapshot()), { status: 200 })
  }))
  renderConfiguration()
  const domain = await screen.findByLabelText('Domain')
  await user.clear(domain)
  await user.type(domain, 'edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Save General' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  expect(await screen.findByText(/domain: Domain is already used/)).toBeVisible()
})

it('can disable an enabled OAuth provider and sends its provider endpoint', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  let patchPath = ''
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') { patchPath = String(input); return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-oauth', revision: 5 }), { status: 202 }) }
    const snapshot = redactedSnapshot()
    snapshot.configuration.auth.oauth = { google: { enabled: true, clientId: 'client-id', secretSet: true, secret: { action: '' }, fields: {} } }
    return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('oauth')
  const toggle = await screen.findByRole('switch', { name: 'Enable Google' })
  await user.click(toggle)
  await user.click(screen.getByRole('button', { name: 'Save Google' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patchPath).toContain('/configuration/oauth/google'))
})
