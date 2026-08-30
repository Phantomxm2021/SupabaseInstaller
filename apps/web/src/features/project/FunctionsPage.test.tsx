import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
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
  await user.upload(await screen.findByLabelText('ZIP archive'), archive)
  await user.click(screen.getByRole('button', { name: 'Deploy function' }))

  await waitFor(() => expect(requests).toContain('/api/projects/bee/functions/hello-world/deploy'))
  await waitFor(() => expect(operationReads).toBeGreaterThan(0))
  await waitFor(() => expect(functionReads).toBeGreaterThan(1))
  expect(await screen.findByText('Function operation complete')).toBeVisible()
  expect(await screen.findByText('hello-world')).toBeVisible()
})
