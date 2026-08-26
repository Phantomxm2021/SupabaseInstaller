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
  expect(screen.getByRole('progressbar')).toHaveAttribute('data-slot', 'progress')
  expect(screen.getByRole('status')).toHaveTextContent('Start Auth')
})

it('awaits project invalidation and navigates exactly once with the operation project id', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'op-success', projectId: 'allocated-project', type: 'CREATE', status: 'SUCCEEDED', progress: 100 }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  vi.stubGlobal('EventSource', class { close() {} addEventListener() {} } as unknown as typeof EventSource)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let release!: () => void
  const invalidation = new Promise<void>((resolve) => { release = resolve })
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries').mockReturnValue(invalidation)
  const onSucceeded = vi.fn()
  render(<QueryClientProvider client={queryClient}><MemoryRouter initialEntries={['/projects']}><Routes><Route path="*" element={<OperationPanel operationId="op-success" projectName="Bee" onSucceeded={onSucceeded} />} /></Routes></MemoryRouter></QueryClientProvider>)
  await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ['projects'] }))
  expect(onSucceeded).not.toHaveBeenCalled()
  release()
  await waitFor(() => expect(onSucceeded).toHaveBeenCalledWith('allocated-project'))
  expect(invalidate).toHaveBeenCalledTimes(1)
  expect(onSucceeded).toHaveBeenCalledTimes(1)
})

it.each(['FAILED', 'ROLLED_BACK', 'CANCELLED'] as const)('does not navigate for terminal %s', async (status) => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: `op-${status}`, projectId: 'bee', type: 'CREATE', status, progress: 100 }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  vi.stubGlobal('EventSource', class { close() {} addEventListener() {} } as unknown as typeof EventSource)
  function Location() { return <output data-testid="location">{useLocation().pathname}</output> }
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects']}><Routes><Route path="*" element={<><Location /><OperationPanel operationId={`op-${status}`} projectName="Bee" /></>} /></Routes></MemoryRouter></QueryClientProvider>)
  await screen.findByText(status)
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/projects'))
})
