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

it('associates every setup field with a description and its validation error', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn())
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}><MemoryRouter><SetupPage /></MemoryRouter></QueryClientProvider>)
  const username = screen.getByLabelText('Username')
  const password = screen.getByLabelText('Password')
  const confirm = screen.getByLabelText('Confirm password')
  expect(username).toHaveAttribute('aria-describedby')
  expect(password).toHaveAttribute('aria-describedby')
  expect(confirm).toHaveAttribute('aria-describedby')
  await user.type(username, 'x')
  await user.type(password, 'short')
  await user.type(confirm, 'different')
  await user.click(screen.getByRole('button', { name: 'Create administrator' }))
  for (const field of [username, password, confirm]) {
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field.getAttribute('aria-describedby')).toMatch(/error/)
  }
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

it('uses shadcn field semantics with an accessible password description', () => {
  vi.stubGlobal('fetch', vi.fn())
  render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><SetupPage /></MemoryRouter></QueryClientProvider>)
  expect(screen.getByTestId('setup-card')).toHaveAttribute('data-slot', 'card')
  const password = screen.getByLabelText('Password')
  expect(password).toHaveAttribute('data-slot', 'input')
  expect(password).toHaveAccessibleDescription('Use 12 or more characters.')
})
