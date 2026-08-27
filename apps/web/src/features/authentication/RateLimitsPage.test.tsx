import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, RouterProvider } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthConfig, Services } from '../../api/types'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import { RateLimitsPage } from './RateLimitsPage'
import { createAppRouter } from '../../app/router'
import { defaultConfiguration } from '../projects/projectSchema'

const auth = { ...defaultConfiguration().auth, email: { ...defaultConfiguration().auth.email, confirmEmail: true } } as AuthConfig
const context = { projectId: 'bee', revision: 2, auth, general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' }, services: { auth: true } as Services, requestSave: vi.fn() } as AuthenticationWorkspaceContext

describe('RateLimitsPage', () => {
  it('renders the GoTrue rate limits in screenshot-style labelled rows', () => {
    const { container } = render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><RateLimitsPage context={context} /></MemoryRouter></QueryClientProvider>)
    expect(screen.getByRole('heading', { name: 'Rate Limits' })).toBeVisible()
    expect(container.querySelector('.auth-rate-limits-card')).toBeInTheDocument()
    expect(screen.getByText(/Safeguard against bursts of incoming traffic to prevent abuse and maximize stability/)).toBeVisible()
    expect(screen.getByLabelText('Rate limit for sending emails')).toHaveValue(30)
    expect(screen.getByText('emails/h')).toBeVisible()
    expect(screen.getByLabelText('Rate limit for token refreshes')).toHaveValue(150)
    expect(screen.getAllByText('requests/5 min')).toHaveLength(3)
  })

  it('validates fields then submits a full Auth configuration through the Auth operation flow', async () => {
    const user = userEvent.setup()
    const requestSave = vi.fn()
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><RateLimitsPage context={{ ...context, requestSave }} /></MemoryRouter></QueryClientProvider>)
    const email = screen.getByLabelText('Rate limit for sending emails')
    await user.clear(email)
    await user.type(email, '42')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(requestSave).toHaveBeenCalledTimes(1))
    expect(requestSave.mock.calls[0][0]).toMatchObject({ section: 'auth', value: { rateLimits: { emailSent: 42, tokenRefresh: 150 } } })
  })

  it('sends changed rate limits in the Auth PATCH after the workspace confirmation', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
      const path = String(input)
      if (path.endsWith('/session')) return new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { headers: { 'Content-Type': 'application/json' } })
      if (path.endsWith('/configuration')) return new Response(JSON.stringify({ projectId: 'bee', revision: 7, lastGoodRevision: 7, configuration: defaultConfiguration() }), { headers: { 'Content-Type': 'application/json' } })
      if (path.endsWith('/configuration/auth') && init?.method === 'PATCH') return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-rate-limits', revision: 8 }), { status: 202, headers: { 'Content-Type': 'application/json' } })
      throw new Error(`Unexpected request: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/projects/bee/authentication/rate-limits')
    const router = createAppRouter(new QueryClient({ defaultOptions: { queries: { retry: false } } }))
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><RouterProvider router={router} /></QueryClientProvider>)
    const email = await screen.findByLabelText('Rate limit for sending emails')
    await user.clear(email)
    await user.type(email, '42')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await user.click(await screen.findByRole('button', { name: 'Confirm and apply' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([path, init]) => String(path).endsWith('/configuration/auth') && (init as RequestInit).method === 'PATCH')).toBe(true))
    const patch = fetchMock.mock.calls.find(([path, init]) => String(path).endsWith('/configuration/auth') && (init as RequestInit).method === 'PATCH')
    expect(JSON.parse((patch![1] as RequestInit).body as string)).toMatchObject({ expectedRevision: 7, value: { rateLimits: { emailSent: 42, tokenRefresh: 150 }, mfa: { maxEnrolledFactors: 10 } } })
    router.dispose()
  })
})
