import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '../../app/router'

const configuration = {
  projectId: 'bee', revision: 7, lastGoodRevision: 7,
  configuration: {
    revision: 7,
    general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' },
    services: { auth: true },
    auth: {
      enabled: true, jwtExpiry: 3600, disableSignup: false,
      email: { enabled: true, allowSignup: true, confirmEmail: false, secureEmailChange: false, doubleConfirmChanges: false },
      phone: { enabled: false, provider: '', secretSet: false, secret: { action: '' }, fields: {} },
      anonymousSignIn: false, redirectUrls: [], oauth: {},
      smtp: { enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' },
    },
    storage: { backend: 'local', s3CompatibleApi: false, bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' }, forcePathStyle: false, localPath: '' },
    realtime: { maxConnections: 100, databasePoolSize: 5, logLevel: 'info' }, functions: { defaultJwtVerification: true, directory: './functions', variables: [] },
    database: { version: '15', directPort: false, directPortNumber: 0, maxConnections: 100, sharedBuffers: '', extensions: [] },
    pooler: { transactionPort: 0, sessionPort: 0, poolSize: 20, maxClientConnections: 100 },
    network: { gateway: 'envoy', httpsMode: 'external', internalGatewayPort: 0, apiPort: 0, studioPort: 0, directDatabasePort: 0, poolerPort: 0 },
  },
}

function renderSignInProviders() {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
    const path = String(input)
    if (path.endsWith('/session')) return new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/configuration')) return new Response(JSON.stringify(configuration), { headers: { 'Content-Type': 'application/json' } })
    if (path.includes('/configuration/oauth/google')) return new Response(JSON.stringify({ projectId: 'bee', operationId: 'operation-1', revision: 8 }), { headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${path} ${init?.method ?? 'GET'}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.pushState({}, '', '/projects/bee/authentication/sign-in-providers')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
  return { fetchMock, router }
}

it('opens Google in a Sheet and saves only that provider', async () => {
  const { fetchMock, router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  expect(screen.getByRole('dialog', { name: 'Google' })).toBeVisible()
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'google-client')
  await user.type(screen.getByLabelText('Google client secret'), 'secret')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/configuration/oauth/google'), expect.objectContaining({ method: 'PATCH' }))
  router.dispose()
})

it('asks before discarding dirty Sheet changes', async () => {
  const { router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  await user.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.getByRole('alertdialog', { name: 'Discard changes?' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Keep editing' }))
  expect(screen.getByRole('dialog', { name: 'Google' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Close' }))
  await user.click(screen.getByRole('button', { name: 'Discard changes' }))
  expect(screen.queryByRole('dialog', { name: 'Google' })).not.toBeInTheDocument()
  router.dispose()
})
