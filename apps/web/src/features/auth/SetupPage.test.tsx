import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SetupPage } from './SetupPage'

it('creates the first administrator and displays recovery codes once', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ recoveryCodes: Array.from({ length: 10 }, (_, index) => `code-${index}`) }), { status: 201, headers: { 'Content-Type': 'application/json' } })))
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <MemoryRouter><SetupPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  await user.type(screen.getByLabelText('Username'), 'admin')
  await user.type(screen.getByLabelText('Password'), 'correct horse battery staple')
  await user.type(screen.getByLabelText('Confirm password'), 'correct horse battery staple')
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))

  expect(await screen.findByRole('heading', { name: 'Save your recovery codes' })).toBeVisible()
  expect(screen.getAllByRole('listitem')).toHaveLength(10)
})

it('explains the minimum password length instead of showing a generic error', async () => {
  const fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <MemoryRouter><SetupPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  await user.type(screen.getByLabelText('Username'), 'admin')
  await user.type(screen.getByLabelText('Password'), '12345678')
  await user.type(screen.getByLabelText('Confirm password'), '12345678')
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))

  expect(await screen.findByText('Password must contain at least 12 characters')).toBeVisible()
  expect(screen.queryByText('Invalid input')).not.toBeInTheDocument()
  expect(fetchMock).not.toHaveBeenCalled()
})
