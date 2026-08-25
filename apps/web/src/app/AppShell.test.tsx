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

  await user.click(screen.getByRole('button', { name: 'Sign out' }))

  expect(method).toBe('DELETE')
})
