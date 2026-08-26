import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AppShell } from './AppShell'
import { AuthenticatedShell } from './router'

it('signs out through the API', async () => {
  let method = ''
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    method = init?.method ?? ''
    return new Response(null, { status: 204 })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects']}><AppShell /></MemoryRouter></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: /account/i }))
  await user.click(await screen.findByRole('menuitem', { name: /sign out/i }))

  expect(method).toBe('DELETE')
})

it('shows Projects in global navigation without a duplicate New Project action', async () => {
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects']}><AppShell /></MemoryRouter></QueryClientProvider>)

  const navigation = screen.getByRole('navigation', { name: /main navigation/i })
  expect(navigation).toBeInTheDocument()
  const projects = screen.getByRole('link', { name: /projects/i })
  expect(projects).toBeInTheDocument()
  expect(projects).toHaveAttribute('aria-current', 'page')
  expect(projects).toHaveAttribute('data-active', '')
  expect(screen.queryByRole('link', { name: /new project/i })).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: /account/i }))
  expect(await screen.findByRole('menuitem', { name: /manager settings/i })).toHaveAttribute('href', '/settings')
})

it('provides a focusable sidebar trigger for narrow screens', () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(screen.getByRole('button', { name: /open sidebar/i })).toBeInTheDocument()
})

it('opens the mobile Sheet and exposes the named navigation landmark', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(screen.queryByRole('navigation', { name: /main navigation/i })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /open sidebar/i }))
  expect(await screen.findByRole('navigation', { name: /main navigation/i })).toBeVisible()
  expect(screen.getByRole('link', { name: /projects/i })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument()
})

it('keeps Sidebar and SidebarInset as direct provider children', () => {
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><AppShell /></MemoryRouter></QueryClientProvider>)
  const wrapper = document.querySelector('[data-slot="sidebar-wrapper"]')
  expect(wrapper?.children).toHaveLength(2)
  expect(wrapper?.children[0]).toHaveAttribute('data-slot', 'sidebar')
  expect(wrapper?.children[1]).toHaveAttribute('data-slot', 'sidebar-inset')
})

it('refreshes CSRF and uses the refreshed token when signing out', async () => {
  const responses = [
    new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'token-one' }), { status: 200 }),
    new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'token-two' }), { status: 200 }),
    new Response(null, { status: 204 }),
  ]
  const requests: RequestInit[] = []
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { requests.push(init ?? {}); return responses.shift()! }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={queryClient}><MemoryRouter initialEntries={['/']}><Routes><Route path="/" element={<AuthenticatedShell />} /><Route path="/login" element={<div>login</div>} /></Routes></MemoryRouter></QueryClientProvider>)
  await screen.findByRole('button', { name: /account/i })
  await queryClient.refetchQueries({ queryKey: ['session'] })
  await userEvent.setup().click(screen.getByRole('button', { name: /account/i }))
  await userEvent.setup().click(await screen.findByRole('menuitem', { name: /sign out/i }))
  expect((requests[2].headers as Headers).get('X-CSRF-Token')).toBe('token-two')
})
