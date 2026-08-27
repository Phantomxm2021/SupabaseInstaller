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
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/settings']}><AppShell /></MemoryRouter></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: /account/i }))
  await user.click(await screen.findByRole('menuitem', { name: /sign out/i }))

  expect(method).toBe('DELETE')
})

it('keeps the projects landing page free of a duplicate sidebar action', async () => {
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects']}><AppShell /></MemoryRouter></QueryClientProvider>)

  expect(screen.queryByRole('navigation', { name: /main navigation/i })).not.toBeInTheDocument()
  expect(screen.queryByRole('link', { name: /new project/i })).not.toBeInTheDocument()
})

it('does not render a sidebar on the projects landing page', () => {
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects']}><AppShell /></MemoryRouter></QueryClientProvider>)

  expect(document.querySelector('[data-slot="sidebar"]')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /sidebar/i })).not.toBeInTheDocument()
})

it.each(['/projects/new', '/projects/bee/configuration'])('does not render a sidebar during %s', (path) => {
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={[path]}><AppShell /></MemoryRouter></QueryClientProvider>)

  expect(document.querySelector('[data-slot="sidebar"]')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /sidebar/i })).not.toBeInTheDocument()
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
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)
  const wrapper = document.querySelector('[data-slot="sidebar-wrapper"]')
  expect(wrapper?.children).toHaveLength(2)
  expect(wrapper?.children[0]).toHaveAttribute('data-slot', 'sidebar')
  expect(wrapper?.children[1]).toHaveAttribute('data-slot', 'sidebar-inset')
})

it('renders project navigation in the single global Sidebar on project routes', () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)
  const sidebar = document.querySelector('[data-slot="sidebar"][data-state]')
  expect(document.querySelectorAll('[data-slot="sidebar"]')).toHaveLength(1)
  expect(sidebar).toHaveAttribute('data-state', 'collapsed')
  expect(document.querySelectorAll('[data-slot="sidebar-gap"]')).toHaveLength(1)
  const projectNavigation = screen.getByRole('navigation', { name: /project navigation/i })
  const expectedLinks = [
    ['Project Overview', '/projects/bee/overview'],
    ['Authentication', '/projects/bee/authentication'],
    ['Project Settings', '/projects/bee/configuration'],
  ] as const
  expect(within(projectNavigation).getAllByRole('link')).toHaveLength(expectedLinks.length)
  for (const [name, href] of expectedLinks) expect(within(projectNavigation).getByRole('link', { name })).toHaveAttribute('href', href)
  expect(within(projectNavigation).getByRole('link', { name: 'Project Overview' })).toHaveClass('primary-sidebar-menu-button')
  expect(within(projectNavigation).getByRole('link', { name: 'Project Overview' })).toHaveAttribute('aria-current', 'page')
  expect(new Set(Array.from(within(projectNavigation).getAllByRole('link')).map((link) => link.getAttribute('href'))).size).toBe(expectedLinks.length)
})

it('uses the same global Sidebar Sheet for project navigation on mobile', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 500 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(screen.queryByRole('navigation', { name: /project navigation/i })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open sidebar' }))
  expect(await screen.findByRole('navigation', { name: /project navigation/i })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Project Settings' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Close' })).toBeVisible()
  expect(document.querySelector('[data-slot="sidebar-trigger"]')).toHaveAttribute('aria-label', 'Close sidebar')
  expect(document.querySelector('[data-slot="sidebar"][data-mobile="true"][data-open]')).toBeVisible()
})

it('starts the desktop sidebar collapsed so it can expand as an overlay on hover', () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(document.querySelector('[data-slot="sidebar"][data-state="collapsed"]')).toBeInTheDocument()
})

it('collapses the primary sidebar after a project tab is clicked', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)

  const sidebar = document.querySelector('[data-slot="sidebar"][data-state]')
  expect(sidebar).toHaveAttribute('data-state', 'collapsed')
  await user.click(screen.getByRole('link', { name: 'Authentication' }))
  expect(sidebar).toHaveAttribute('data-state', 'collapsed')
})

it('renders a page-wide dashboard header above the project navigation shell', () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/authentication/users']}><AppShell /></MemoryRouter></QueryClientProvider>)

  const header = screen.getByRole('banner', { name: 'Dashboard header' })
  expect(header).toHaveClass('topbar')
  expect(header).toHaveTextContent('bee')
  expect(screen.getByRole('button', { name: 'Show projects' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Show branches' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Show organizations' })).not.toBeInTheDocument()
})

it('switches projects from the header menu and routes creation through the existing wizard', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ projects: [
    { id: 'bee', name: 'BeeGame', slug: 'bee', domain: 'bee.local', siteUrl: '', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'v2', preset: 'DEFAULT', configurationRevision: 1, services: {}, createdAt: '', updatedAt: '' },
    { id: 'other', name: 'Other project', slug: 'other', domain: 'other.local', siteUrl: '', status: 'RUNNING', health: 'HEALTHY', supabaseVersion: 'v2', preset: 'DEFAULT', configurationRevision: 1, services: {}, createdAt: '', updatedAt: '' },
  ] }), { status: 200 })))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: 'Show projects' }))
  expect(await screen.findByPlaceholderText('Find project...')).toBeVisible()
  expect(await screen.findByRole('menuitem', { name: 'Other project' })).toHaveAttribute('href', '/projects/other/overview')
  expect(screen.getByRole('menuitem', { name: 'New project' })).toHaveAttribute('href', '/projects/new')
})

it('opens the header branch menu with the current local main branch selected', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={['/projects/bee/overview']}><AppShell /></MemoryRouter></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: 'Show branches' }))
  expect(await screen.findByRole('menuitemradio', { name: /main.*local/i })).toHaveAttribute('aria-checked', 'true')
})

it.each([
  ['/projects/bee/overview', 'Project Overview'],
  ['/projects/bee/authentication/emails', 'Authentication'],
] as const)('marks only the canonical active link for %s', (path, activeLabel) => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={[path]}><AppShell /></MemoryRouter></QueryClientProvider>)
  const sidebar = document.querySelector('[data-slot="sidebar"][data-state]')
  const activeLinks = within(sidebar as HTMLElement).getAllByRole('link').filter((link) => link.getAttribute('aria-current') === 'page')
  expect(activeLinks).toHaveLength(1)
  expect(activeLinks[0]).toHaveAccessibleName(activeLabel)
})

it.each(['/projects', '/projects/new', '/settings'])('does not render project navigation at %s', (path) => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1024 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter initialEntries={[path]}><AppShell /></MemoryRouter></QueryClientProvider>)
  expect(screen.queryByRole('navigation', { name: /project navigation/i })).not.toBeInTheDocument()
})

it.each([
  ['/projects/bee/authentication/sign-in-providers', 'Authentication'],
] as const)('highlights the canonical project section for %s', (path, activeLabel) => {
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
