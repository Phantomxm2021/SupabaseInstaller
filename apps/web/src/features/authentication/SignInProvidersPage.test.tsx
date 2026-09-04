import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RouterProvider } from 'react-router-dom'
import { createAppRouter } from '../../app/router'
import { defaultMailerConfiguration } from '../projects/projectSchema'

function configuration(revision: number, googleEnabled = false, oauthOverrides: Record<string, unknown> = {}) { return {
  projectId: 'bee', revision, lastGoodRevision: revision,
  configuration: {
    revision,
    general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' },
    services: { auth: true },
    auth: {
      enabled: true, jwtExpiry: 3600, disableSignup: false,
      email: { enabled: true, allowSignup: true, confirmEmail: false, secureEmailChange: false, doubleConfirmChanges: false },
      phone: { enabled: false, provider: '', secretSet: false, secret: { action: '' }, fields: {} },
      anonymousSignIn: false, redirectUrls: [], oauth: { ...(googleEnabled ? { google: { enabled: true, clientId: 'google-client', secretSet: true, secret: { action: '' }, fields: {} } } : {}), ...oauthOverrides },
      smtp: { enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' },
      mailer: defaultMailerConfiguration(),
      rateLimits: { emailSent: 30, smsSent: 30, tokenRefresh: 150, tokenVerification: 30, anonymousUsers: 30, signupsAndSignins: 30 },
      mfa: { totpEnrollEnabled: true, totpVerifyEnabled: true, phoneEnrollEnabled: false, phoneVerifyEnabled: false, maxEnrolledFactors: 10, phoneOtpLength: 6 },
    },
    storage: { backend: 'local', s3CompatibleApi: false, bucket: '', region: '', endpoint: '', accountId: '', accessKeyId: '', secretAccessKeySet: false, secretAccessKey: { action: '' }, forcePathStyle: false, uploadFileSizeLimit: 50 * 1024 * 1024, localPath: '' },
    realtime: { maxConnections: 100, databasePoolSize: 5, logLevel: 'info' }, functions: { defaultJwtVerification: true, variables: [] },
    database: { version: '15', directPort: false, directPortNumber: 0, maxConnections: 100, sharedBuffers: '', extensions: [] },
    pooler: { transactionPort: 0, sessionPort: 0, poolSize: 20, maxClientConnections: 100 },
    network: { gateway: 'envoy', httpsMode: 'external', internalGatewayPort: 0, apiPort: 0, studioPort: 0, directDatabasePort: 0, poolerPort: 0 },
  },
} }

function renderSignInProviders(oauthOverrides: Record<string, unknown> = {}) {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let revision = 7
  let googleEnabled = false
  const fetchMock = vi.fn(async (input: string | URL, init?: RequestInit) => {
    const path = String(input)
    if (path.endsWith('/session')) return new Response(JSON.stringify({ username: 'admin', mustChangePassword: false, csrfToken: 'csrf-token' }), { headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/configuration')) return new Response(JSON.stringify(configuration(revision, googleEnabled, oauthOverrides)), { headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/configuration/auth')) { revision += 1; return new Response(JSON.stringify({ projectId: 'bee', operationId: 'auth-operation', revision }), { headers: { 'Content-Type': 'application/json' } }) }
    if (path.includes('/configuration/oauth/google')) { revision += 1; googleEnabled = true; return new Response(JSON.stringify({ projectId: 'bee', operationId: 'operation-1', revision }), { headers: { 'Content-Type': 'application/json' } }) }
    if (path.endsWith('/operations/auth-operation')) return new Response(JSON.stringify({ id: 'auth-operation', projectId: 'bee', type: 'UPDATE_CONFIG', status: 'SUCCEEDED', currentStep: 'MARK_CONFIGURATION_GOOD', progress: 100 }), { headers: { 'Content-Type': 'application/json' } })
    if (path.endsWith('/operations/operation-1')) return new Response(JSON.stringify({ id: 'operation-1', projectId: 'bee', type: 'UPDATE_CONFIG', status: 'SUCCEEDED', currentStep: 'MARK_CONFIGURATION_GOOD', progress: 100 }), { headers: { 'Content-Type': 'application/json' } })
    throw new Error(`Unexpected request: ${path} ${init?.method ?? 'GET'}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.pushState({}, '', '/projects/bee/authentication/sign-in-providers')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(queryClient)
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
  return { fetchMock, router }
}

it('edits JWT expiry from the active Authentication settings', async () => {
  const { fetchMock, router } = renderSignInProviders()
  const user = userEvent.setup()

  const input = await screen.findByRole('spinbutton', { name: 'JWT expiry (seconds)' })
  expect(input).toHaveAttribute('min', '1')
  expect(input).toHaveAttribute('max', '604800')
  await user.clear(input)
  await user.type(input, '7200')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))

  await waitFor(() => expect(fetchMock.mock.calls.some(([path, init]) => String(path).endsWith('/configuration/auth') && (init as RequestInit).method === 'PATCH')).toBe(true))
  const patchCall = fetchMock.mock.calls.find(([path, init]) => String(path).endsWith('/configuration/auth') && (init as RequestInit).method === 'PATCH')
  expect(JSON.parse((patchCall?.[1] as RequestInit).body as string).value.jwtExpiry).toBe(7200)
  router.dispose()
})

it('saves only Google with a replacement secret then refetches its revision', async () => {
  const { fetchMock, router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  expect(screen.getByRole('dialog', { name: 'Google' })).toBeVisible()
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  await user.click(screen.getByRole('switch', { name: 'Skip nonce checks' }))
  await user.type(screen.getByLabelText('Client IDs'), 'google-client')
  await user.type(screen.getByLabelText('Client Secret (for OAuth)'), 'secret')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(screen.getByRole('button', { name: /Google.*Enabled/i })).toBeVisible())
  const patches = () => fetchMock.mock.calls.filter(([path, init]) => String(path).includes('/configuration/oauth/google') && (init as RequestInit).method === 'PATCH')
  expect(JSON.parse((patches()[0][1] as RequestInit).body as string)).toEqual(expect.objectContaining({ value: expect.objectContaining({ enabled: true, clientId: 'google-client', secret: { action: 'replace', value: 'secret' }, fields: expect.objectContaining({ skipNonceChecks: 'true' }) }) }))
  expect(fetchMock.mock.calls.some(([path]) => String(path).endsWith('/configuration'))).toBe(true)
  await user.click(screen.getByRole('button', { name: /Google.*Enabled/i }))
  await user.type(screen.getByLabelText('Client IDs'), '-2')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patches()).toHaveLength(2))
  expect(JSON.parse((patches()[1][1] as RequestInit).body as string)).toEqual(expect.objectContaining({ value: expect.objectContaining({ clientId: 'google-client-2', secret: { action: 'retain' } }) }))
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
  expect(screen.getByRole('switch', { name: 'Allow manual linking' })).toBeVisible()
  expect(screen.getByRole('button', { name: /Email.*Enabled/i })).toBeVisible()
  expect(screen.getByRole('button', { name: /Google.*Disabled/i })).toBeVisible()

  await user.click(screen.getByRole('tab', { name: 'Third-Party Auth' }))
  expect(screen.getByText(/No separate third-party provider configuration/i)).toBeVisible()
  expect(screen.queryByRole('button', { name: /Google.*Disabled/i })).not.toBeInTheDocument()
  router.dispose()
})

it('uses shared dashboard section gaps between provider headings and cards', async () => {
  const { router } = renderSignInProviders()

  const userSignups = await screen.findByRole('heading', { name: 'User Signups' })
  const authProviders = screen.getByRole('heading', { name: 'Auth Providers' })
  expect(userSignups.closest('form')).toHaveClass('dashboard-section')
  expect(authProviders.closest('section')).toHaveClass('dashboard-section')
  expect(userSignups.closest('[data-slot="tabs-content"]')).toHaveClass('dashboard-stack')
  router.dispose()
})

it('marks provider rows as compact dashboard controls', async () => {
  const { router } = renderSignInProviders()

  const email = await screen.findByRole('button', { name: /Email.*Enabled/i })
  expect(email).toHaveAttribute('data-density', 'dashboard-compact-row')
  router.dispose()
})

it('exposes the official email security and OTP controls in the provider drawer', async () => {
  const { router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Email.*Enabled/i }))
  expect(screen.getByRole('dialog', { name: 'Email' })).toBeVisible()
  expect(screen.getByRole('dialog', { name: 'Email' })).toHaveAttribute('data-provider-drawer', 'true')
  expect(screen.getByRole('switch', { name: 'Secure password change' })).toBeVisible()
  expect(screen.getByRole('switch', { name: 'Require current password when updating' })).toBeVisible()
  expect(screen.getByRole('switch', { name: 'Prevent use of leaked passwords' })).toBeVisible()
  expect(screen.getByRole('spinbutton', { name: 'Minimum password length' })).toHaveValue(6)
  expect(screen.getByRole('spinbutton', { name: 'Email OTP expiration' })).toHaveValue(3600)
  expect(screen.getByRole('spinbutton', { name: 'Email OTP length' })).toHaveValue(8)
  router.dispose()
})

it('exposes Google nonce and missing-email controls with the official labels', async () => {
  const { router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  expect(screen.getByText('Client IDs')).toBeVisible()
  expect(screen.getByRole('switch', { name: 'Skip nonce checks' })).toBeVisible()
  expect(screen.getByRole('switch', { name: 'Allow users without an email' })).toBeVisible()
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  expect(screen.getByRole('button', { name: 'Copy Callback URL (for OAuth)' })).toBeVisible()
  router.dispose()
})

it.each(['Azure', 'GitHub', 'GitLab', 'KeyCloak'])('does not ask to discard unchanged %s provider settings', async (provider) => {
  const { router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: new RegExp(`${provider}.*Disabled`, 'i') }))
  expect(screen.getByRole('dialog', { name: provider })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.queryByRole('alertdialog', { name: 'Discard changes?' })).not.toBeInTheDocument()
  router.dispose()
})

it.each([
  ['Azure', { enabled: true, clientId: 'azure-client', secretSet: true, secret: { action: '' }, fields: { tenantUrl: 'https://login.microsoftonline.com/tenant' } }],
  ['GitHub', { enabled: true, clientId: 'github-client', secretSet: true, secret: { action: '' }, fields: { enterpriseUrl: 'https://github.example.com' } }],
  ['GitLab', { enabled: true, clientId: 'gitlab-client', secretSet: true, secret: { action: '' }, fields: { selfHostedUrl: 'https://gitlab.example.com' } }],
  ['KeyCloak', { enabled: true, clientId: 'keycloak-client', secretSet: true, secret: { action: '' }, fields: { realmUrl: 'https://keycloak.example.com/realms/example' } }],
] as const)('does not mark unchanged special-field %s configuration dirty', async (provider, config) => {
  const { router } = renderSignInProviders({ [provider.toLowerCase()]: config })
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: new RegExp(`${provider}.*Enabled`, 'i') }))
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.queryByRole('alertdialog', { name: 'Discard changes?' })).not.toBeInTheDocument()
  router.dispose()
})

it('resets provider dirty state after discarding one provider before opening another', async () => {
  const { router } = renderSignInProviders()
  const user = userEvent.setup()

  await user.click(await screen.findByRole('button', { name: /Google.*Disabled/i }))
  await user.click(screen.getByRole('switch', { name: 'Enable Google' }))
  await user.click(screen.getByRole('button', { name: 'Close' }))
  await user.click(screen.getByRole('button', { name: 'Discard changes' }))

  await user.click(screen.getByRole('button', { name: /Azure.*Disabled/i }))
  await user.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.queryByRole('alertdialog', { name: 'Discard changes?' })).not.toBeInTheDocument()
  router.dispose()
})
