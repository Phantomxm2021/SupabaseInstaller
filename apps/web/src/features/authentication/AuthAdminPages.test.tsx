import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { createAppRouter } from '../../app/router'
import { defaultConfiguration } from '../projects/projectSchema'

function renderAuthentication(path: string, fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock)
  window.history.pushState({}, '', path)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createAppRouter(queryClient)
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

function workspaceResponse(input: string | URL) {
  const path = String(input)
  if (path.endsWith('/api/session')) return { username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }
  if (path.endsWith('/api/projects/bee/configuration')) return { projectId: 'bee', revision: 1, lastGoodRevision: 1, configuration: defaultConfiguration() }
  return undefined
}

describe('authentication user administration', () => {
  it('renders the live user-list columns', async () => {
    const fetchMock = vi.fn(async (input: string | URL) => {
      const body = workspaceResponse(input)
      if (body) return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/users')) return new Response(JSON.stringify({ users: [{ id: 'c1', email: 'ada@example.test', phone: '+8613000000000', user_metadata: { display_name: 'Ada' }, identities: [{ provider: 'google' }] }] }), { headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${input}`)
    })
    const router = renderAuthentication('/projects/bee/authentication/users', fetchMock)

    expect(await screen.findByRole('heading', { name: 'Users' })).toBeVisible()
    expect(screen.getByRole('link', { name: 'Users' })).toHaveAttribute('aria-current', 'page')
    expect(await screen.findByText('ada@example.test')).toBeVisible()
    expect(screen.getByText('+8613000000000')).toBeVisible()
    expect(screen.getByText('Social')).toBeVisible()
    expect(screen.getByRole('columnheader', { name: 'Provider type' })).toBeVisible()
    router.dispose()
  })

  it('opens the Add user menu and creates a user through the Manager-only admin endpoint', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
      const body = workspaceResponse(input)
      if (body) return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/users') && init?.method === 'POST') return new Response(JSON.stringify({ id: 'new-user', email: 'new@example.test', user_metadata: {}, identities: [{ provider: 'email' }] }), { status: 201, headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/users')) return new Response(JSON.stringify({ users: [] }), { headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${input}`)
    })
    const router = renderAuthentication('/projects/bee/authentication/users', fetchMock)

    await screen.findByRole('heading', { name: 'Users' })
    await user.click(screen.getByRole('button', { name: 'Add user' }))
    await user.click(await screen.findByRole('menuitem', { name: /Create new user/i }))
    await user.type(screen.getByLabelText('Email address'), 'new@example.test')
    await user.type(screen.getByLabelText('User Password'), 'password123')
    await user.click(screen.getByRole('button', { name: 'Create user' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([path, init]) => String(path).includes('/auth/users') && (init as RequestInit).method === 'POST')).toBe(true))
    const request = fetchMock.mock.calls.find(([path, init]) => String(path).includes('/auth/users') && (init as RequestInit).method === 'POST')
    expect(JSON.parse((request![1] as RequestInit).body as string)).toEqual({ email: 'new@example.test', password: 'password123', email_confirm: true })
    router.dispose()
  })

  it('opens the invitation dialog from the Add user menu', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn(async (input: string | URL) => {
      const body = workspaceResponse(input)
      if (body) return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/users')) return new Response(JSON.stringify({ users: [] }), { headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${input}`)
    })
    const router = renderAuthentication('/projects/bee/authentication/users', fetchMock)

    await screen.findByRole('heading', { name: 'Users' })
    await user.click(screen.getByRole('button', { name: 'Add user' }))
    await user.click(await screen.findByRole('menuitem', { name: /Send invitation/i }))
    expect(await screen.findByRole('heading', { name: 'Invite a new user' })).toBeVisible()
    expect(screen.getByLabelText('User email')).toBeVisible()
    router.dispose()
  })

  it('sends an invitation through the Manager-only admin endpoint', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
      const body = workspaceResponse(input)
      if (body) return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/users/invite') && init?.method === 'POST') return new Response(JSON.stringify({ id: 'invitee', email: 'invitee@example.test' }), { status: 201, headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/users')) return new Response(JSON.stringify({ users: [] }), { headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${input}`)
    })
    const router = renderAuthentication('/projects/bee/authentication/users', fetchMock)

    await screen.findByRole('heading', { name: 'Users' })
    await user.click(screen.getByRole('button', { name: 'Add user' }))
    await user.click(await screen.findByRole('menuitem', { name: /Send invitation/i }))
    await user.type(screen.getByLabelText('User email'), 'invitee@example.test')
    await user.click(screen.getByRole('button', { name: 'Invite user' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([path, init]) => String(path).includes('/auth/users/invite') && (init as RequestInit).method === 'POST')).toBe(true))
    const request = fetchMock.mock.calls.find(([path, init]) => String(path).includes('/auth/users/invite') && (init as RequestInit).method === 'POST')
    expect(JSON.parse((request![1] as RequestInit).body as string)).toEqual({ email: 'invitee@example.test' })
    router.dispose()
  })
})

describe('OAuth Apps', () => {
  it('shows the native OAuth Server disabled state returned by GoTrue', async () => {
    const fetchMock = vi.fn(async (input: string | URL) => {
      const body = workspaceResponse(input)
      if (body) return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/oauth-apps')) return new Response(JSON.stringify({ error: { code: 'OAUTH_SERVER_DISABLED', message: 'OAuth Server is disabled for this project' } }), { status: 404, headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${input}`)
    })
    const router = renderAuthentication('/projects/bee/authentication/oauth-apps', fetchMock)

    expect(await screen.findByText('OAuth Server is disabled')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'New OAuth App' })).not.toBeInTheDocument()
    router.dispose()
  })

  it('creates an OAuth app through the Manager-only admin endpoint', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
      const body = workspaceResponse(input)
      if (body) return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/oauth-apps') && init?.method === 'POST') return new Response(JSON.stringify({ client_id: 'app-1', name: 'Dashboard', client_type: 'confidential', redirect_uris: ['https://app.example.test/callback'] }), { status: 201, headers: { 'Content-Type': 'application/json' } })
      if (String(input).includes('/auth/oauth-apps')) return new Response(JSON.stringify({ clients: [] }), { headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${input}`)
    })
    const router = renderAuthentication('/projects/bee/authentication/oauth-apps', fetchMock)

    await screen.findByRole('heading', { name: 'OAuth Apps' })
    await user.click(screen.getByRole('button', { name: 'New OAuth App' }))
    await user.type(screen.getByLabelText('Client name'), 'Dashboard')
    await user.type(screen.getByLabelText('Redirect URI'), 'https://app.example.test/callback')
    await user.click(screen.getByRole('button', { name: 'Create OAuth App' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([path, init]) => String(path).includes('/auth/oauth-apps') && (init as RequestInit).method === 'POST')).toBe(true))
    const request = fetchMock.mock.calls.find(([path, init]) => String(path).includes('/auth/oauth-apps') && (init as RequestInit).method === 'POST')
    expect(JSON.parse((request![1] as RequestInit).body as string)).toEqual({ name: 'Dashboard', redirect_uris: ['https://app.example.test/callback'], client_type: 'confidential', token_endpoint_auth_method: 'client_secret_basic' })
    router.dispose()
  })
})
