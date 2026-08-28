import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach } from 'vitest'
import { NewProjectPage } from './NewProjectPage'

const projectListResponse = (projects: unknown[] = []) => new Response(
  JSON.stringify({ projects }),
  { status: 200, headers: { 'Content-Type': 'application/json' } },
)

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => projectListResponse()))
})

async function waitForIdentityAvailability() {
  await waitFor(() => {
    expect(screen.getByText('Project name is available')).toBeVisible()
    expect(screen.getByText('Project slug is available')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  })
}

it('does not render the setup tab navigation while creating a project', () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(screen.queryByRole('tab', { name: /1\. Basic/ })).not.toBeInTheDocument()
  expect(screen.getByText('Step 1 of 6 · Basic')).toBeVisible()
  expect(screen.getByRole('button', { name: /Continue/ })).toBeVisible()
})

it('shows the identity fields in the Basic step', () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(screen.getByLabelText('Project name')).toBeVisible()
  expect(screen.getByLabelText('Project slug')).toBeVisible()
  expect(screen.getByLabelText('Site URL hostname')).toBeVisible()
  expect(screen.getByLabelText('Studio username')).toBeVisible()
  expect(screen.getByLabelText('Studio password')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Runtime settings' })).toBeVisible()
  expect(screen.queryByLabelText('Pinned Supabase version')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Project URL')).not.toBeInTheDocument()
})

it('shows available project identity feedback before enabling Continue', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  await waitForIdentityAvailability()
  expect(screen.getAllByRole('status')).toHaveLength(2)
  expect(screen.getAllByRole('status')[0]).toHaveAttribute('aria-live', 'polite')
})

it('associates identity validation errors with their inputs', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  const name = screen.getByLabelText('Project name')
  const slug = screen.getByLabelText('Project slug')
  await user.type(name, 'x'.repeat(81))
  await user.clear(slug)
  await user.type(slug, 'Invalid slug')

  await waitFor(() => {
    expect(name).toHaveAttribute('aria-invalid', 'true')
    expect(slug).toHaveAttribute('aria-invalid', 'true')
  })
  expect(name).toHaveAttribute('aria-describedby', 'name-form-item-message')
  expect(slug).toHaveAttribute('aria-describedby', 'slug-form-item-message')
  expect(document.getElementById('name-form-item-message')).toBeVisible()
  expect(document.getElementById('slug-form-item-message')).toBeVisible()
})

it('blocks a duplicate project identity before progression', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => projectListResponse([{ name: 'Production API', slug: 'production-api' }])))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  expect(await screen.findByText('A project named “Production API” already exists')).toBeVisible()
  expect(screen.getByText('The slug “production-api” is already in use')).toBeVisible()
  expect(screen.getByLabelText('Project name')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByLabelText('Project slug')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('shows a retryable availability check failure instead of a duplicate', async () => {
  let attempts = 0
  vi.stubGlobal('fetch', vi.fn(async () => {
    attempts += 1
    if (attempts === 1) return new Response(JSON.stringify({ error: { message: 'Projects unavailable' } }), { status: 503, headers: { 'Content-Type': 'application/json' } })
    return projectListResponse()
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  expect(await screen.findByText('Could not check project name availability. Try again.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.click(screen.getAllByRole('button', { name: 'Retry' })[0])
  await waitForIdentityAvailability()
})

it('installs Lightweight after name and base site URL', async () => {
  let createBody: Record<string, unknown> = {}
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    createBody = JSON.parse(String(init?.body)) as Record<string, unknown>
    return new Response(JSON.stringify({ projectId: 'project-1', operationId: 'operation-1' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  await user.type(screen.getByLabelText('Project name'), 'Production API')
  expect(screen.getByLabelText('Project slug')).toHaveValue('production-api')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Review' }))
  expect(screen.getByText('Lightweight')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Install project' }))

  expect(createBody.preset).toBe('LIGHTWEIGHT')
  expect(screen.getByRole('heading', { level: 1, name: 'Installing Production API' })).toBeVisible()
})

it('prefixes the Basic-step Site URL with HTTPS before submitting', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init?.body))
    return new Response(JSON.stringify({ projectId: 'project-https', operationId: 'operation-https' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Project name'), 'Production API')
  expect(screen.getByText('https://')).toBeVisible()
  await user.type(screen.getByLabelText('Site URL hostname'), 'app.example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Review' }))
  await user.click(screen.getByRole('button', { name: 'Install project' }))

  await waitFor(() => expect(body?.configuration.general.siteUrl).toBe('https://app.example.com'))
})

it('collects Studio username and password during project creation', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init?.body))
    return new Response(JSON.stringify({ projectId: 'project-studio', operationId: 'operation-studio' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  expect(screen.getByLabelText('Studio username')).toBeVisible()
  expect(screen.getByLabelText('Studio password')).toHaveAttribute('type', 'password')
  await user.clear(screen.getByLabelText('Studio username'))
  await user.type(screen.getByLabelText('Studio username'), 'admin')
  await user.type(screen.getByLabelText('Studio password'), 'strong-password')
  await waitForIdentityAvailability()
  for (let index = 0; index < 5; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install project' }))
  await waitFor(() => expect(body?.configuration.general).toMatchObject({ studioUsername: 'admin', studioPassword: { action: 'replace', value: 'strong-password' } }))
})

it('blocks the Review shortcut when required Basic fields are invalid', async () => {
  const fetchSpy = vi.fn()
  vi.stubGlobal('fetch', fetchSpy)
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  expect(screen.getByRole('button', { name: 'Review' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  expect(fetchSpy).toHaveBeenCalledWith('/api/projects', expect.anything())
})

it('posts the complete aggregate after navigating every wizard step', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { if (!init?.method || init.method === 'GET') return projectListResponse(); body = JSON.parse(String(init?.body)); return new Response(JSON.stringify({ projectId: 'project-2', operationId: 'operation-2' }), { status: 202, headers: { 'Content-Type': 'application/json' } }) }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 5; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install project' }))
  await waitFor(() => expect(body?.configuration).toBeDefined())
  expect(body?.supabaseVersion).toBeUndefined()
  expect(body?.configuration.auth.email.secureEmailChange).toBe(false)
  expect(body?.configuration.realtime).toEqual({ maxConnections: 100, databasePoolSize: 5, logLevel: 'info' })
  expect(body?.configuration.storage.forcePathStyle).toBe(false)
  expect(body?.configuration.pooler.maxClientConnections).toBe(100)
  expect(body?.configuration.pooler.transactionPort).toBe(0)
  expect(body?.configuration.pooler.sessionPort).toBe(0)
  expect(body?.configuration.services.database).toBe(true)
  expect(body).not.toHaveProperty('domain')
  expect(body).not.toHaveProperty('siteUrl')
  expect(body).not.toHaveProperty('services')
})

it('uses Standard aggregate controls and closes Direct DB through Custom without a hard-coded port', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { if (!init?.method || init.method === 'GET') return projectListResponse(); body = JSON.parse(String(init?.body)); return new Response(JSON.stringify({ projectId: 'project-3', operationId: 'operation-3' }), { status: 202, headers: { 'Content-Type': 'application/json' } }) }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Standard' }))
  expect(screen.getByRole('switch', { name: 'Edge Functions' })).toBeChecked()
  expect(screen.getByRole('switch', { name: 'Storage' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'Direct PostgreSQL port' }))
  expect(screen.getByRole('switch', { name: 'Direct PostgreSQL port' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install project' }))
  await waitFor(() => expect(body?.configuration).toBeDefined())
  expect(body?.preset).toBe('CUSTOM')
  expect(body?.configuration.services.functions).toBe(true)
  expect(body?.configuration.services.directDb).toBe(true)
  expect(body?.configuration.database.directPort).toBe(true)
  expect(body?.configuration.database.directPortNumber).toBe(0)
  expect(body?.configuration.network.directDatabasePort).toBe(0)
})

it('restores the full dependency closure when a feature is enabled again', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'Authentication' }))
  await user.click(screen.getByRole('switch', { name: 'PostgREST' }))
  await user.click(screen.getByRole('switch', { name: 'Supabase Studio' }))
  await user.click(screen.getByRole('switch', { name: 'API Gateway' }))
  expect(screen.getByRole('switch', { name: 'API Gateway' })).not.toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Edge Functions' }))
  expect(screen.getByRole('switch', { name: /^API Gateway/ })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^PostgreSQL/ })).toBeChecked()
})

it('enabling Storage or Image Transformation atomically restores database, REST, and gateway', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'PostgREST' }))
  expect(screen.getByRole('switch', { name: 'PostgREST' })).not.toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Storage' }))
  expect(screen.getByRole('switch', { name: 'Storage' })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^PostgREST/ })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^API Gateway/ })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^PostgreSQL/ })).toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Image Transformation' }))
  expect(screen.getByRole('switch', { name: 'Image Transformation' })).toBeChecked()
})

it('renders nested OAuth secret value errors at the secret control', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Project name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'client')
  await user.type(screen.getByLabelText('Client secret'), ' ')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByText('A replacement value is required')).toBeVisible()
})
