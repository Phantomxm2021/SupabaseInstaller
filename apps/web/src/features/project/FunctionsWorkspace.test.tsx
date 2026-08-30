import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '@/app/router'

it('renders Deployments and Secrets in the Functions secondary sidebar', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input)
    if (path.endsWith('/api/session')) return new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/api/projects')) return new Response(JSON.stringify({ projects: [{ id: 'bee', name: 'Bee', slug: 'bee', domain: 'bee.local', siteUrl: '', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'v2', preset: 'DEFAULT', configurationRevision: 1, services: {}, createdAt: '', updatedAt: '' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/api/projects/bee/functions')) return new Response(JSON.stringify({ functions: [], enabled: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${path}`)
  }))
  window.history.pushState({}, '', '/projects/bee/functions')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)

  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)

  expect(await screen.findByRole('navigation', { name: 'Functions navigation' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Deployments' })).toHaveAttribute('aria-current', 'page')
  expect(screen.getByRole('link', { name: 'Secrets' })).toHaveAttribute('href', '/projects/bee/functions/secrets')
  expect(screen.queryByRole('tablist', { name: 'Functions navigation' })).not.toBeInTheDocument()
  router.dispose()
})
