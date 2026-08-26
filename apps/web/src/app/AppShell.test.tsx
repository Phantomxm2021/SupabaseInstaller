import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
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

it('renders project navigation in the single global Sidebar on project routes', () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/configuration?section=services']}><AppShell /></MemoryRouter></QueryClientProvider>)
  const sidebar = document.querySelector('[data-slot="sidebar"][data-state]')
  expect(document.querySelectorAll('[data-slot="sidebar"]')).toHaveLength(1)
  expect(sidebar).toHaveAttribute('data-state', 'expanded')
  expect(document.querySelectorAll('[data-slot="sidebar-gap"]')).toHaveLength(1)
  const projectNavigation = screen.getByRole('navigation', { name: /project navigation/i })
  const expectedLinks = [
    ['Overview', '/projects/bee/overview'], ['General', '/projects/bee/configuration?section=general'], ['Services', '/projects/bee/configuration?section=services'],
    ['Authentication', '/projects/bee/configuration?section=auth'], ['Email & SMTP', '/projects/bee/configuration?section=smtp'], ['OAuth Providers', '/projects/bee/configuration?section=oauth'],
    ['Storage', '/projects/bee/configuration?section=storage'], ['Realtime', '/projects/bee/configuration?section=realtime'], ['Functions', '/projects/bee/configuration?section=functions'],
    ['Database', '/projects/bee/configuration?section=database'], ['Connection Pool', '/projects/bee/configuration?section=pooler'], ['Gateway & Network', '/projects/bee/configuration?section=network'], ['API & Secrets', '/projects/bee/configuration?section=secrets'],
  ] as const
  expect(within(projectNavigation).getAllByRole('link')).toHaveLength(expectedLinks.length)
  for (const [name, href] of expectedLinks) expect(within(projectNavigation).getByRole('link', { name })).toHaveAttribute('href', href)
  expect(within(projectNavigation).getByRole('link', { name: 'Services' })).toHaveAttribute('aria-current', 'page')
  expect(new Set(Array.from(within(projectNavigation).getAllByRole('link')).map((link) => link.getAttribute('href'))).size).toBe(expectedLinks.length)
  const runtimeNavigation = screen.getByRole('navigation', { name: /runtime navigation/i })
  expect(within(runtimeNavigation).getByRole('link', { name: 'Logs' })).toHaveAttribute('href', '/projects/bee/logs')
  expect(within(runtimeNavigation).getByRole('link', { name: 'Backups' })).toHaveAttribute('href', '/projects/bee/backups')
})

it('uses the same global Sidebar Sheet for project navigation on mobile', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(screen.queryByRole('navigation', { name: /project navigation/i })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open sidebar' }))
  expect(await screen.findByRole('navigation', { name: /project navigation/i })).toBeVisible()
  expect(screen.getByRole('link', { name: 'General' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Close' })).toBeVisible()
  expect(document.querySelector('[data-slot="sidebar-trigger"]')).toHaveAttribute('aria-label', 'Close sidebar')
  expect(document.querySelector('[data-slot="sidebar"][data-mobile="true"][data-open]')).toBeVisible()
})

it('updates the global desktop trigger name as the Sidebar changes state', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects']}><AppShell /></MemoryRouter></QueryClientProvider>)
  await user.click(screen.getByRole('button', { name: 'Close sidebar' }))
  expect(screen.getByRole('button', { name: 'Open sidebar' })).toBeVisible()
})

it.each(['/projects', '/projects/new', '/settings'])('does not render project navigation at %s', (path) => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={[path]}><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(screen.queryByRole('navigation', { name: /project navigation/i })).not.toBeInTheDocument()
})

it.each([
  ['/projects/bee/configuration', 'General'],
  ['/projects/bee/configuration?section=unknown', 'General'],
  ['/projects/bee/configuration?section=oauth', 'OAuth Providers'],
] as const)('highlights the canonical configuration section for %s', (path, activeLabel) => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={[path]}><AppShell /></MemoryRouter></QueryClientProvider>)
  const projectNavigation = screen.getByRole('navigation', { name: /project navigation/i })
  expect(within(projectNavigation).getByRole('link', { name: activeLabel })).toHaveAttribute('aria-current', 'page')
  expect(within(projectNavigation).getAllByRole('link').filter((link) => link.getAttribute('aria-current') === 'page')).toHaveLength(1)
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
