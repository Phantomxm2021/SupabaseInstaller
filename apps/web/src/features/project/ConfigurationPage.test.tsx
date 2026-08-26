import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ConfigurationPage } from './ConfigurationPage'
import { defaultConfiguration } from '../projects/projectSchema'

it('renders the installed project configuration workspace from the redacted snapshot', async () => {
  const configuration = defaultConfiguration('LIGHTWEIGHT')
  configuration.general = { domain: 'bee.example.com', siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' }
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ projectId: 'bee', revision: 4, lastGoodRevision: 4, configuration }), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={['/projects/bee/configuration?section=database']}><Routes><Route path="/projects/:projectId/configuration" element={<ConfigurationPage />} /></Routes></MemoryRouter></QueryClientProvider>)
  expect(await screen.findByText('Database', { selector: '[data-slot="card-title"]' })).toBeVisible()
  expect(screen.getByLabelText('Maximum connections')).toBeVisible()
  expect(screen.getByRole('tab', { name: 'API & Secrets' })).toBeVisible()
  expect(screen.getByRole('tablist')).toHaveClass('h-auto')
  expect(screen.getByRole('tablist')).toHaveClass('overflow-visible')
})

function redactedSnapshot(domain = 'bee.example.com') {
  const configuration = defaultConfiguration('LIGHTWEIGHT')
  configuration.general = { domain, siteUrl: 'https://example.com', supabaseVersion: 'self-hosted/v0.8.0' }
  const redacted = JSON.parse(JSON.stringify(configuration)) as typeof configuration
  delete (redacted.auth as unknown as { redirectUrls?: string[] }).redirectUrls
  delete (redacted.auth as unknown as { oauth?: Record<string, unknown> }).oauth?.google
  delete (redacted.auth.phone as unknown as { provider?: string }).provider
  delete (redacted.auth.phone as unknown as { fields?: Record<string, string> }).fields
  delete (redacted.functions as unknown as { variables?: unknown[] }).variables
  delete (redacted.database as unknown as { extensions?: string[] }).extensions
  return { projectId: 'bee', revision: 4, lastGoodRevision: 4, configuration: redacted }
}

function renderConfiguration(section = 'general') {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MemoryRouter initialEntries={[`/projects/bee/configuration?section=${section}`]}><Routes><Route path="/projects/:projectId/configuration" element={<ConfigurationPage />} /></Routes></MemoryRouter></QueryClientProvider>)
}

it('keeps dirty input when preview is dismissed with Keep editing', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-1', revision: 5 }), { status: 202 })
    return new Response(JSON.stringify(redactedSnapshot()), { status: 200 })
  }))
  renderConfiguration()
  const domain = await screen.findByLabelText('Domain')
  await user.clear(domain)
  await user.type(domain, 'edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Save General' }))
  expect(await screen.findByRole('alertdialog')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Keep editing' }))
  expect(screen.getByLabelText('Domain')).toHaveValue('edited.example.com')
})

it('preserves dirty fields on 409 and only Reload resets to new server data', async () => {
  const user = userEvent.setup()
  let getCount = 0
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ error: { code: 'CONFIGURATION_STALE', message: 'stale' } }), { status: 409 })
    getCount += 1
    return new Response(JSON.stringify(redactedSnapshot(getCount > 1 ? 'server.example.com' : 'bee.example.com')), { status: 200 })
  }))
  renderConfiguration()
  const domain = await screen.findByLabelText('Domain')
  await user.clear(domain)
  await user.type(domain, 'edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Save General' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  expect(await screen.findByText('This configuration is stale. Your dirty fields are preserved.')).toBeVisible()
  expect(screen.getByLabelText('Domain')).toHaveValue('edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Reload' }))
  await waitFor(() => expect(screen.getByLabelText('Domain')).toHaveValue('server.example.com'))
})

it('renders authoritative API field errors', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ error: { code: 'INVALID_CONFIGURATION', message: 'invalid', fields: { domain: 'Domain is already used' } } }), { status: 422 })
    return new Response(JSON.stringify(redactedSnapshot()), { status: 200 })
  }))
  renderConfiguration()
  const domain = await screen.findByLabelText('Domain')
  await user.clear(domain)
  await user.type(domain, 'edited.example.com')
  await user.click(screen.getByRole('button', { name: 'Save General' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  expect(await screen.findByText(/domain: Domain is already used/)).toBeVisible()
  expect(screen.getByLabelText('Domain')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText('Domain is already used')).toBeVisible()
})

it('can disable an enabled OAuth provider and sends its provider endpoint', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  let patchPath = ''
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') { patchPath = String(input); return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-oauth', revision: 5 }), { status: 202 }) }
    const snapshot = redactedSnapshot()
    snapshot.configuration.auth.oauth = { google: { enabled: true, clientId: 'client-id', secretSet: true, secret: { action: '' }, fields: {} } }
    return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('oauth')
  const toggle = await screen.findByRole('switch', { name: 'Enable Google' })
  await user.click(toggle)
  await user.click(screen.getByRole('button', { name: 'Save Google' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patchPath).toContain('/configuration/oauth/google'))
})

it('associates nested OAuth API field errors with the provider control', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ error: { code: 'INVALID_CONFIGURATION', message: 'invalid', fields: { 'auth.oauth.google.clientId': 'Client ID rejected' } } }), { status: 422 })
    const snapshot = redactedSnapshot(); snapshot.configuration.auth.oauth = { google: { enabled: true, clientId: 'client-id', secretSet: true, secret: { action: '' }, fields: {} } }; return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('oauth')
  await user.click(await screen.findByRole('switch', { name: 'Enable Google' }))
  await user.click(screen.getByRole('button', { name: 'Save Google' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  const providerForm = document.getElementById('configuration-oauth-google-form')
  expect(providerForm).not.toBeNull()
  const clientId = within(providerForm as HTMLElement).getByLabelText('Client ID')
  expect(clientId).toHaveAttribute('aria-invalid', 'true')
  expect(clientId).toHaveAttribute('aria-describedby')
  expect(screen.getByText('Client ID rejected')).toBeVisible()
})

it('keeps removal reachable for disabled configured SMTP and previews no runtime action', async () => {
  const user = userEvent.setup()
  let patchBody = ''
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') { patchBody = String(init.body); return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-smtp', revision: 5 }), { status: 202 }) }
    const snapshot = redactedSnapshot(); snapshot.configuration.auth.smtp = { enabled: false, host: '', port: 587, username: '', passwordSet: true, password: { action: '' }, senderEmail: '', senderName: '' }; return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('smtp')
  await screen.findByRole('button', { name: 'Remove Password' })
  await user.click(screen.getByRole('button', { name: 'Remove Password' }))
  await user.click(screen.getByRole('button', { name: 'Save Email & SMTP' }))
  expect(await screen.findByText('No runtime restart expected')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patchBody).toContain('"action":"remove"'))
})

it('closes only public dependents when Gateway is disabled', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  let patchBody = ''
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') { patchBody = String(init.body); return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-services', revision: 5 }), { status: 202 }) }
    const snapshot = redactedSnapshot()
    snapshot.configuration.services = { ...defaultConfiguration('FULL').services, supavisor: true, logs: true, vector: true, directDb: true }
    return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('services')
  await user.click(await screen.findByRole('switch', { name: 'Envoy Gateway' }))
  expect(screen.getByRole('switch', { name: 'Auth' })).toHaveAttribute('data-unchecked')
  expect(screen.getByRole('switch', { name: 'PostgREST' })).toHaveAttribute('data-unchecked')
  expect(screen.getByRole('switch', { name: 'Supavisor' })).toHaveAttribute('data-checked')
  expect(screen.getByRole('switch', { name: 'Logs / Logflare' })).toHaveAttribute('data-checked')
  expect(screen.getByRole('switch', { name: 'Direct PostgreSQL port' })).toHaveAttribute('data-checked')
  expect(screen.getByRole('switch', { name: 'postgres-meta' })).toHaveAttribute('data-checked')
  await user.click(screen.getByRole('button', { name: 'Save Services' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patchBody).toContain('"postgresMeta":true'))
})

it('enables Studio with Gateway and postgres-meta, then persists the closure', async () => {
  const user = userEvent.setup(); let patchBody = ''
  vi.stubGlobal('PointerEvent', MouseEvent)
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') { patchBody = String(init.body); return new Response(JSON.stringify({ projectId: 'bee', operationId: 'op-studio', revision: 5 }), { status: 202 }) }
    const snapshot = redactedSnapshot(); snapshot.configuration.services = { ...defaultConfiguration('LIGHTWEIGHT').services, gateway: false, studio: false, postgresMeta: true }; return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('services')
  await user.click(await screen.findByRole('switch', { name: 'Studio' }))
  expect(screen.getByRole('switch', { name: 'Envoy Gateway' })).toHaveAttribute('data-checked')
  expect(screen.getByRole('switch', { name: 'postgres-meta' })).toHaveAttribute('data-checked')
  await user.click(screen.getByRole('button', { name: 'Save Services' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  await waitFor(() => expect(patchBody).toContain('"gateway":true'))
  expect(patchBody).toContain('"postgresMeta":true')
})

it.each([
  ['general', 'Domain', 'edited.example.com', 'Save General'],
  ['network', 'Gateway', 'Kong (advanced)', 'Save Gateway & Network'],
  ['pooler', 'Pool size', '21', 'Save Connection Pooler'],
] as const)('renders metadata-only preview for disabled %s owner after editing', async (section, field, value, saveLabel) => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  vi.stubGlobal('fetch', vi.fn(async () => { const snapshot = redactedSnapshot(); snapshot.configuration.services = { ...defaultConfiguration('LIGHTWEIGHT').services, gateway: false, auth: false, studio: false, supavisor: false }; return new Response(JSON.stringify(snapshot), { status: 200 }) }))
  renderConfiguration(section)
  if (section === 'network') {
    const gateway = await screen.findByRole('combobox', { name: 'Gateway' })
    await user.click(gateway)
    await waitFor(() => expect(screen.getByRole('listbox')).toBeVisible())
    await user.click(screen.getByText('Kong (advanced)'))
  } else {
    const control = await screen.findByLabelText(field)
    await user.clear(control); await user.type(control, value)
  }
  await user.click(screen.getByRole('button', { name: saveLabel }))
  expect(await screen.findByText('Configuration metadata only')).toBeVisible()
  expect(screen.getByText('No runtime restart expected')).toBeVisible()
})

it('shows an accessible error on the Phone provider when phone auth blocks submit', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(redactedSnapshot()), { status: 200, headers: { 'Content-Type': 'application/json' } })))
  renderConfiguration('auth')
  const phone = await screen.findByRole('switch', { name: 'Enable Phone Auth' })
  await user.click(phone)
  await user.click(screen.getByRole('button', { name: 'Save Authentication' }))
  const provider = screen.getByRole('combobox', { name: 'Phone provider' })
  expect(provider).toHaveAttribute('aria-invalid', 'true')
  expect(provider).toHaveAttribute('aria-describedby')
  expect(screen.getByText('Choose a supported phone provider')).toBeVisible()
})

it('shows disableSignup client and nested server errors on the associated toggle', async () => {
  const user = userEvent.setup(); vi.stubGlobal('PointerEvent', MouseEvent)
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PATCH') return new Response(JSON.stringify({ error: { code: 'INVALID_CONFIGURATION', message: 'invalid', fields: { 'auth.disableSignup': 'Signup is required by enabled providers' } } }), { status: 422 })
    const snapshot = redactedSnapshot(); snapshot.configuration.auth.phone.enabled = true; snapshot.configuration.auth.phone.provider = 'twilio'; snapshot.configuration.auth.phone.secretSet = true; snapshot.configuration.auth.phone.secret = { action: '' }; snapshot.configuration.auth.phone.fields = { accountSid: 'a', messageServiceSid: 'm' }; return new Response(JSON.stringify(snapshot), { status: 200 })
  }))
  renderConfiguration('auth')
  await user.click(await screen.findByRole('switch', { name: 'Anonymous sign-in' }))
  await user.click(screen.getByRole('button', { name: 'Save Authentication' }))
  await user.click(screen.getByRole('button', { name: 'Confirm and apply' }))
  const keepEditing = screen.queryByRole('button', { name: 'Keep editing' })
  if (keepEditing) await user.click(keepEditing)
  const signup = await screen.findByRole('switch', { name: 'Allow signup' })
  expect(signup).toHaveAttribute('aria-invalid', 'true')
  expect(signup).toHaveAttribute('aria-describedby')
  const errorId = signup.getAttribute('aria-describedby')
  expect(errorId).toBeTruthy()
  expect(document.getElementById(errorId as string)).toHaveTextContent('Signup is required by enabled providers')
})
