import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '../../app/router'

it('renders sign-in providers in the authentication workspace without configuration tabs', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  window.history.pushState({}, '', '/projects/bee/authentication/sign-in-providers')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)

  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)

  expect(await screen.findByRole('heading', { name: 'Sign In / Providers' })).toBeVisible()
  expect(screen.getByRole('navigation', { name: 'Authentication navigation' })).toBeVisible()
  expect(screen.queryByRole('tablist', { name: /configuration/i })).not.toBeInTheDocument()
  router.dispose()
})

it('keeps project navigation and highlights the active Authentication item', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  window.history.pushState({}, '', '/projects/bee/authentication/emails')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)

  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)

  expect(await screen.findByRole('link', { name: 'Overview' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Emails' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByText('NOTIFICATIONS')).toBeVisible()
  router.dispose()
})

it('opens the Authentication navigation from an accessible mobile trigger', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 700 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  window.history.pushState({}, '', '/projects/bee/authentication/emails')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)
  const user = userEvent.setup()

  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)

  const trigger = await screen.findByRole('button', { name: 'Open authentication navigation' })
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await user.click(trigger)
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(await screen.findByRole('navigation', { name: 'Authentication navigation' })).toBeVisible()
  router.dispose()
})

it('renders an explicit placeholder for unsupported Authentication routes', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  window.history.pushState({}, '', '/projects/bee/authentication/sessions')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)

  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)

  expect(await screen.findByRole('heading', { name: 'Sessions' })).toBeVisible()
  expect(screen.getByText('Not configured in this Manager version')).toBeVisible()
  router.dispose()
})
