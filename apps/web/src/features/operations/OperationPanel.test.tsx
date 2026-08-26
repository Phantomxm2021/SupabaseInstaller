import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { OperationPanel } from './OperationPanel'

it('shows the failed step and offers recovery actions', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'op-1', projectId: 'bee', type: 'CREATE', status: 'FAILED', currentStep: 'START_AUTH', progress: 70, errorMessage: 'Auth unhealthy' }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  vi.stubGlobal('EventSource', class { close() {} addEventListener() {} } as unknown as typeof EventSource)
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><OperationPanel operationId="op-1" projectName="Bee" /></QueryClientProvider>)

  expect(await screen.findByText('Start Auth')).toBeVisible()
  expect(screen.getByText('Auth unhealthy')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  expect(screen.getByRole('button', { name: 'Rollback' })).toBeEnabled()
})

it('awaits project invalidation and navigates exactly once with the operation project id', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'op-success', projectId: 'allocated-project', type: 'CREATE', status: 'SUCCEEDED', progress: 100 }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  vi.stubGlobal('EventSource', class { close() {} addEventListener() {} } as unknown as typeof EventSource)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue(undefined)
  function Location() { return <output data-testid="location">{useLocation().pathname}</output> }
  render(<QueryClientProvider client={queryClient}><MemoryRouter initialEntries={['/projects']}><Routes><Route path="*" element={<><Location /><OperationPanel operationId="op-success" projectName="Bee" /></>} /></Routes></MemoryRouter></QueryClientProvider>)
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/projects/allocated-project/overview'))
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ['projects'] })
  expect(screen.getByTestId('location')).toHaveTextContent('/projects/allocated-project/overview')
})

it.each(['FAILED', 'ROLLED_BACK', 'CANCELLED'] as const)('does not navigate for terminal %s', async (status) => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: `op-${status}`, projectId: 'bee', type: 'CREATE', status, progress: 100 }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  vi.stubGlobal('EventSource', class { close() {} addEventListener() {} } as unknown as typeof EventSource)
  function Location() { return <output data-testid="location">{useLocation().pathname}</output> }
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects']}><Routes><Route path="*" element={<><Location /><OperationPanel operationId={`op-${status}`} projectName="Bee" /></>} /></Routes></MemoryRouter></QueryClientProvider>)
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/projects'))
})
