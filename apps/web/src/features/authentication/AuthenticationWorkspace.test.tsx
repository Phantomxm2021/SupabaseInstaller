import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
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
