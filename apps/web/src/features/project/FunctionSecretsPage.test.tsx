import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { defaultConfiguration } from '../projects/projectSchema'
import { FunctionSecretsPage } from './FunctionSecretsPage'

it('updates a function secret without rendering its replacement value', async () => {
  const user = userEvent.setup()
  const configuration = defaultConfiguration('LIGHTWEIGHT')
  configuration.functions.variables = [{ name: 'STRIPE_KEY', valueSet: true, value: { action: '' } }]
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    if (path === '/api/projects/bee/configuration/functions' && init?.method === 'PATCH') return new Response(JSON.stringify({ projectId: 'bee', operationId: 'functions-op' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
    if (path === '/api/projects/bee/configuration') return new Response(JSON.stringify({ projectId: 'bee', revision: 4, lastGoodRevision: 4, configuration }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    if (path === '/api/operations/functions-op') return new Response(JSON.stringify({ id: 'functions-op', projectId: 'bee', type: 'UPDATE_CONFIG', status: 'QUEUED', progress: 10 }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/functions/secrets']}><Routes><Route path="/projects/:projectId/functions/secrets" element={<FunctionSecretsPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  expect(await screen.findByRole('heading', { name: 'Functions' })).toBeVisible()
  expect(screen.getByDisplayValue('STRIPE_KEY')).toBeVisible()
  expect(screen.getByText('Configured')).toBeVisible()
  const secret = screen.getByLabelText('Value for STRIPE_KEY')
  expect(secret).not.toHaveValue('stored-secret')
  await user.type(secret, 'replacement-secret')
  await user.click(screen.getByRole('button', { name: 'Save Functions' }))
  await user.click(await screen.findByRole('button', { name: 'Confirm and apply' }))

  await waitFor(() => expect(fetchMock.mock.calls.some(([path, init]) => String(path) === '/api/projects/bee/configuration/functions' && (init as RequestInit).method === 'PATCH')).toBe(true))
  const [, request] = fetchMock.mock.calls.find(([path, init]) => String(path) === '/api/projects/bee/configuration/functions' && (init as RequestInit).method === 'PATCH') as [string, RequestInit]
  expect(JSON.parse(String(request.body))).toMatchObject({ value: { variables: [{ name: 'STRIPE_KEY', value: { action: 'replace', value: 'replacement-secret' } }] } })
  expect(screen.queryByText('replacement-secret')).not.toBeInTheDocument()
})
