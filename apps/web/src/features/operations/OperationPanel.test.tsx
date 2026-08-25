import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
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
