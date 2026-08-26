import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AppShell } from './AppShell'

it('signs out through the API', async () => {
  let method = ''
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    method = init?.method ?? ''
    return new Response(null, { status: 204 })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><AppShell /></MemoryRouter></QueryClientProvider>)

  await user.click(screen.getByRole('button', { name: /account/i }))
  await user.click(await screen.findByRole('menuitem', { name: /sign out/i }))

  expect(method).toBe('DELETE')
})

it('shows Projects in global navigation without a duplicate New Project action', async () => {
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><AppShell /></MemoryRouter></QueryClientProvider>)

  expect(screen.getByRole('link', { name: /projects/i })).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: /new project/i })).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: /account/i }))
  expect(await screen.findByRole('menuitem', { name: /manager settings/i })).toHaveAttribute('href', '/settings')
})
