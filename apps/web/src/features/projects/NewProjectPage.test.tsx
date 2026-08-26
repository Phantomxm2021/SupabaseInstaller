import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { NewProjectPage } from './NewProjectPage'

it('installs Lightweight after name, domain, and site URL', async () => {
  let createBody: Record<string, unknown> = {}
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    createBody = JSON.parse(String(init?.body)) as Record<string, unknown>
    return new Response(JSON.stringify({ projectId: 'project-1', operationId: 'operation-1' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  await user.type(screen.getByLabelText('Project name'), 'Bee')
  expect(screen.getByLabelText('Project slug')).toHaveValue('bee')
  await user.type(screen.getByLabelText('Domain'), 'bee.example.com')
  await user.type(screen.getByLabelText('Site URL'), 'https://example.com')
  await user.click(screen.getByRole('button', { name: 'Review' }))
  expect(screen.getByText('Lightweight')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Install project' }))

  expect(createBody.preset).toBe('LIGHTWEIGHT')
  expect(screen.getByRole('heading', { level: 1, name: 'Installing Bee' })).toBeVisible()
})

it('blocks the Review shortcut when required Basic fields are invalid', async () => {
  const fetchSpy = vi.fn()
  vi.stubGlobal('fetch', fetchSpy)
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: 'Review' }))
  expect(screen.getByText('Project name is required')).toBeVisible()
  expect(screen.getByLabelText('Project name')).toHaveAttribute('aria-invalid', 'true')
  expect(fetchSpy).not.toHaveBeenCalled()
})

it('posts the complete aggregate after navigating every wizard step', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { body = JSON.parse(String(init?.body)); return new Response(JSON.stringify({ projectId: 'project-2', operationId: 'operation-2' }), { status: 202, headers: { 'Content-Type': 'application/json' } }) }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Project name'), 'Bee')
  await user.type(screen.getByLabelText('Domain'), 'bee.example.com')
  await user.type(screen.getByLabelText('Site URL'), 'https://example.com')
  for (let index = 0; index < 5; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install project' }))
  await waitFor(() => expect(body?.configuration).toBeDefined())
  expect(body?.configuration.auth.email.secureEmailChange).toBe(false)
  expect(body?.configuration.realtime).toEqual({ maxConnections: 100, databasePoolSize: 5, logLevel: 'info' })
  expect(body?.configuration.storage.forcePathStyle).toBe(false)
  expect(body?.configuration.pooler.maxClientConnections).toBe(100)
  expect(body?.services.database).toBe(true)
})
