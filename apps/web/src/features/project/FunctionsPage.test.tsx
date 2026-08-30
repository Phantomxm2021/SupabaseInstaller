import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { FunctionsPage } from './FunctionsPage'

it('tracks a queued deployment and refreshes the function list after it succeeds', async () => {
  let functionReads = 0
  let operationReads = 0
  const requests: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)
    requests.push(path)
    if (init?.method === 'POST') {
      return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-deploy' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
    }
    if (path === '/api/operations/op-deploy') {
      operationReads += 1
      return new Response(JSON.stringify({ id: 'op-deploy', projectId: 'bee', type: 'DEPLOY_FUNCTION', status: 'SUCCEEDED', progress: 100 }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    if (path === '/api/projects/bee/functions') {
      functionReads += 1
      const functions = functionReads > 1 ? [{ name: 'hello-world', current: { sha256: 'abcdef1234567890', operationId: 'op-deploy', deployedAt: '2026-08-31T00:00:00Z' } }] : []
      return new Response(JSON.stringify({ functions, enabled: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    }
    throw new Error(`Unexpected request: ${path}`)
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/functions']}><Routes><Route path="/projects/:projectId/functions" element={<FunctionsPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  const archive = new File(['zip-body'], 'hello-world.zip', { type: 'application/zip' })
  await user.click(await screen.findByRole('button', { name: 'Deploy a new function' }))
  await user.click(await screen.findByRole('button', { name: 'Choose ZIP file' }))
  await user.upload(await screen.findByLabelText('Function ZIP file'), archive)
  expect(screen.getByText('hello-world.zip')).toBeVisible()
  expect(screen.getByText('ZIP archive ready to deploy')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Deploy function' }))

  await waitFor(() => expect(requests).toContain('/api/projects/bee/functions/hello-world/deploy'))
  await waitFor(() => expect(operationReads).toBeGreaterThan(0))
  await waitFor(() => expect(functionReads).toBeGreaterThan(1))
  expect(await screen.findByText('Deployment complete')).toBeVisible()
  expect(screen.getByRole('dialog')).toBeVisible()
  expect(screen.queryByLabelText('Function ZIP file')).not.toBeInTheDocument()
  expect(await screen.findByText('hello-world')).toBeVisible()

  await user.click(within(screen.getByRole('dialog')).getAllByRole('button', { name: 'Close' })[0])
  expect(screen.queryByText('Deployment complete')).not.toBeInTheDocument()
  expect(screen.queryByText('Finalizing function deployment')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Deploy a new function' }))

  expect(await screen.findByLabelText('Function ZIP file')).toBeVisible()
})

it('does not render Functions navigation as in-page tabs', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === '/api/projects/bee/functions') return new Response(JSON.stringify({ functions: [], enabled: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${input}`)
  }))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/functions']}><Routes><Route path="/projects/:projectId/functions" element={<FunctionsPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  await screen.findByText('Managed functions')
  expect(screen.queryByRole('tablist', { name: 'Functions navigation' })).not.toBeInTheDocument()
})

it('opens the deployment dialog from the page header', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === '/api/projects/bee/functions') return new Response(JSON.stringify({ functions: [], enabled: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${input}`)
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/functions']}><Routes><Route path="/projects/:projectId/functions" element={<FunctionsPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  await user.click(await screen.findByRole('button', { name: 'Deploy a new function' }))

  expect(await screen.findByRole('heading', { name: 'Deploy a function' })).toBeVisible()
  expect(screen.getByLabelText('Function name')).toBeVisible()
  expect(screen.getByLabelText('Function ZIP file')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Choose ZIP file' })).toBeVisible()
})

it('opens the deployment dialog with the selected managed function name', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === '/api/projects/bee/functions') return new Response(JSON.stringify({ functions: [{ name: 'hello-world', current: { sha256: 'abcdef1234567890', operationId: 'op-deploy', deployedAt: '2026-08-31T00:00:00Z' } }], enabled: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${input}`)
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/functions']}><Routes><Route path="/projects/:projectId/functions" element={<FunctionsPage />} /></Routes></MemoryRouter></QueryClientProvider>)

  await user.click(await screen.findByRole('button', { name: 'Actions for hello-world' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Deploy new version' }))

  expect(await screen.findByRole('heading', { name: 'Deploy a function' })).toBeVisible()
  expect(await screen.findByLabelText('Function name')).toHaveValue('hello-world')
})
