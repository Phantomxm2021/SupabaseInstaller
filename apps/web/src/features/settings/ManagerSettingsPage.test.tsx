import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ManagerSettingsPage } from './ManagerSettingsPage'

it('renders safe account and control-plane fields without exposing CSRF data', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
    username: 'admin',
    mustChangePassword: true,
    csrfToken: 'do-not-render-this',
  }), { status: 200, headers: { 'Content-Type': 'application/json' } })))

  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><ManagerSettingsPage /></MemoryRouter></QueryClientProvider>)

  expect(await screen.findByText('admin')).toBeInTheDocument()
  expect(screen.getByText(/change your password/i)).toBeInTheDocument()
  expect(screen.queryByText('csrfToken')).not.toBeInTheDocument()
  expect(screen.queryByText('do-not-render-this')).not.toBeInTheDocument()
})
