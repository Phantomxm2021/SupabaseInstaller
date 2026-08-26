import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '../../app/router'

function configuration(revision: number, googleEnabled = false) { return {
  projectId: 'bee', revision, lastGoodRevision: revision,
  configuration: {
    revision,
    general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' },
    services: { auth: true },
    auth: {
      enabled: true, jwtExpiry: 3600, disableSignup: false,
      email: { enabled: true, allowSignup: true, confirmEmail: false, secureEmailChange: false, doubleConfirmChanges: false },
      phone: { enabled: false, provider: '', secretSet: false, secret: { action: '' }, fields: {} },
      anonymousSignIn: false, redirectUrls: [], oauth: googleEnabled ? { google: { enabled: true, clientId: 'google-client', secretSet: true, secret: { action: '' }, fields: {} } } : {},
      smtp: { enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' },
      rateLimits: { emailSent: 30, smsSent: 30, tokenRefresh: 150, tokenVerification: 30, anonymousUsers: 30, signupsAndSignins: 30 },
      mfa: { totpEnrollEnabled: true, totpVerifyEnabled: true, phoneEnrollEnabled: false, phoneVerifyEnabled: false, maxEnrolledFactors: 10, phoneOtpLength: 6 },
    },
    storage: { backend: 'local', s3CompatibleApi: false, bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' }, forcePathStyle: false, localPath: '' },
    realtime: { maxConnections: 100, databasePoolSize: 5, logLevel: 'info' }, functions: { defaultJwtVerification: true, directory: './functions', variables: [] },
    database: { version: '15', directPort: false, directPortNumber: 0, maxConnections: 100, sharedBuffers: '', extensions: [] },
    pooler: { transactionPort: 0, sessionPort: 0, poolSize: 20, maxClientConnections: 100 },
    network: { gateway: 'envoy', httpsMode: 'external', internalGatewayPort: 0, apiPort: 0, studioPort: 0, directDatabasePort: 0, poolerPort: 0 },
  },
} }

function renderSignInProviders() {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let revision = 7
  let googleEnabled = false
  const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
    const path = String(input)
    if (path.endsWith('/session')) return new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/configuration')) return new Response(JSON.stringify(configuration(revision, googleEnabled)), { headers: { 'Content-Type': 'application/json' } })
    if (path.includes('/configuration/oauth/google')) { revision += 1; googleEnabled = true; return new Response(JSON.stringify({ projectId: 'bee', operationId: 'operation-1', revision }), { headers: { 'Content-Type': 'application/json' } }) }
    throw new Error(`Unexpected request: ${path} ${init?.method ?? 'GET'}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.pushState({}, '', '/projects/bee/authentication/sign-in-providers')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
  return { fetchMock, router }
}

it('saves only Google with a replacement secret then refetches its revision', async () => {
  const { fetchMock, router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  expect(screen.getByRole('dialog', { name: 'Google' })).toBeVisible()
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'google-client')
  await user.type(screen.getByLabelText('Google client secret'), 'secret')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(screen.getByRole('button', { name: /Google.*Enabled/i })).toBeVisible())
  const patches = () => fetchMock.mock.calls.filter(([path, init]) => String(path).includes('/configuration/oauth/google') && (init as RequestInit).method === 'PATCH')
  expect(JSON.parse((patches()[0][1] as RequestInit).body as string)).toEqual(expect.objectContaining({ expectedRevision: 7, value: expect.objectContaining({ enabled: true, clientId: 'google-client', secret: { action: 'replace', value: 'secret' } }) }))
  expect(fetchMock.mock.calls.some(([path]) => String(path).endsWith('/configuration'))).toBe(true)
  await user.click(screen.getByRole('button', { name: /Google.*Enabled/i }))
  await user.type(screen.getByLabelText('Client ID'), '-2')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patches()).toHaveLength(2))
  expect(JSON.parse((patches()[1][1] as RequestInit).body as string)).toEqual(expect.objectContaining({ expectedRevision: 8, value: expect.objectContaining({ clientId: 'google-client-2', secret: { action: 'retain' } }) }))
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

it('separates built-in and OAuth provider browsing with the Supabase Cloud tab layout', async () => {
  const { router } = renderSignInProviders()
  const user = userEvent.setup()

  expect(await screen.findByRole('tab', { name: 'Supabase Auth' })).toBeVisible()
  expect(screen.getByRole('tab', { name: 'Third-Party Auth' })).toBeVisible()
  expect(screen.getByRole('button', { name: /Email.*Enabled/i })).toBeVisible()
  expect(screen.getByRole('button', { name: /Google.*Disabled/i })).toBeVisible()

  await user.click(screen.getByRole('tab', { name: 'Third-Party Auth' }))
  expect(screen.getByText(/No separate third-party provider configuration/i)).toBeVisible()
  expect(screen.queryByRole('button', { name: /Google.*Disabled/i })).not.toBeInTheDocument()
  router.dispose()
})
