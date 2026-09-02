import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach } from 'vitest'
import { NewProjectPage } from './NewProjectPage'

const projectListResponse = (projects: unknown[] = []) => new Response(
  JSON.stringify({ projects }),
  { status: 200, headers: { 'Content-Type': 'application/json' } },
)

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => projectListResponse()))
})

async function waitForIdentityAvailability() {
  await waitFor(() => {
    expect(screen.getByText('Server name is available')).toBeVisible()
    expect(screen.getByText('Server slug is available')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled()
  })
}

it('does not render the setup tab navigation while creating a project', () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(screen.queryByRole('tab', { name: /1\. Basic/ })).not.toBeInTheDocument()
  expect(screen.getByText('Step 1 of 4 · Server details')).toBeVisible()
  expect(screen.getByRole('button', { name: /Continue/ })).toBeVisible()
})

it('keeps TLS configuration in a dedicated second card on the server details step', async () => {
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(screen.getByText('TLS certificate')).toBeVisible()
  expect(screen.getByText('Use default certificate')).toBeVisible()
  await user.click(screen.getByText('Upload custom certificate'))
  expect(screen.getByLabelText('Certificate (.pem or .crt)')).toBeVisible()
  expect(screen.getByLabelText('Private key (.key or .pem)')).toBeVisible()
})

it('disables directional transforms when reduced motion is requested', () => {
  const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

  expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\) \{\s+\.wizard-step-frame\[data-direction\] \{\s+animation-duration: 1ms !important;\s+animation-name: wizard-step-fade;\s+transform: none;/)
})

it('uses the primary foreground for the selected preset description', () => {
  const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

  expect(styles).toMatch(/\.service-preset-option\[aria-current="true"\] \.service-preset-description \{\s+color: var\(--primary-foreground\);/)
})

it('stacks preset labels above descriptions and stretches the desktop preset separator', () => {
  const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

  expect(styles).toMatch(/\.service-preset-option \{\s+width: 100%;\s+height: auto;\s+display: flex;\s+flex-direction: column;/)
  expect(styles).toMatch(/\.service-preset-nav \{\s+display: flex;\s+flex-direction: column;[\s\S]*?align-self: stretch;/)
  expect(styles).toMatch(/@media \(max-width: 700px\) \{\s+\.service-configuration-layout \{[\s\S]*?\}\s+\.service-preset-nav \{\s+padding: 0 0 12px;\s+border-right: 0;\s+border-bottom: 1px solid var\(--border\);/)
})

it('moves through four steps with directional motion and focuses the next heading', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(screen.getByText('Step 2 of 4 · Services')).toBeVisible()
  expect(screen.getByTestId('wizard-step-frame')).toHaveAttribute('data-direction', 'forward')
  expect(screen.getByRole('heading', { level: 1, name: 'Create a server' })).toHaveFocus()
  await user.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByTestId('wizard-step-frame')).toHaveAttribute('data-direction', 'backward')
  expect(screen.getByText('Step 1 of 4 · Server details')).toBeVisible()
})

it('groups services under a persistent preset navigation', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(screen.getByRole('navigation', { name: 'Service presets' })).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Core services' })).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Extended services' })).toBeVisible()
  expect(screen.getByText(/6 of 14 services enabled/)).toBeVisible()
  expect(screen.getByRole('button', { name: 'Lightweight' })).toHaveAttribute('aria-current', 'true')
})

it('applies Standard services and keeps Custom selected after a service edit', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Standard' }))

  expect(screen.getByRole('button', { name: 'Standard' })).toHaveAttribute('aria-current', 'true')
  expect(screen.getByRole('switch', { name: 'Storage' })).toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Edge Functions' }))
  expect(screen.getByRole('button', { name: 'Custom' })).toHaveAttribute('aria-current', 'true')
  expect(screen.getByText('Custom keeps your service choices when you continue editing.')).toBeVisible()
})

it('explains service dependency closure in the grouped configuration', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(screen.getByText('API Gateway is required by enabled services.')).toBeVisible()
  await user.click(screen.getByRole('switch', { name: 'Storage' }))
  expect(screen.getByText('Storage requires PostgREST; disabling Storage also disables Image Transformation.')).toBeVisible()
  await user.click(screen.getByRole('switch', { name: 'Logs & Analytics' }))
  expect(screen.getAllByText('Logs & Analytics and Vector are enabled or disabled together.')).toHaveLength(2)
})

it('keeps an invalid integrations step visible and focuses its first invalid control', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'client')
  await user.type(screen.getByLabelText('Client secret'), ' ')
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  const invalid = screen.getByLabelText('Client secret')
  expect(screen.getByText('Step 3 of 4 · Security & integrations')).toBeVisible()
  expect(invalid).toHaveAttribute('aria-invalid', 'true')
  expect(invalid).toHaveFocus()
})

it('adds and removes OAuth providers through the authentication provider picker', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'GitHub' }))

  expect(screen.getByText('GitHub')).toBeVisible()
  expect(screen.queryByRole('switch', { name: 'Enable GitHub' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Google' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Remove GitHub' }))
  expect(screen.getByRole('heading', { name: 'Remove GitHub?' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Remove' }))
  expect(screen.queryByText('GitHub')).not.toBeInTheDocument()
})

it('renders only enabled security integration module bodies', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  const authenticationSwitch = screen.getByRole('switch', { name: 'Authentication' })
  const providerPicker = screen.getByRole('button', { name: 'Add authentication provider' })
  expect(authenticationSwitch).toBeChecked()
  expect(providerPicker).toBeVisible()
  expect(providerPicker.compareDocumentPosition(authenticationSwitch) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  await user.click(screen.getByRole('switch', { name: 'Authentication' }))
  expect(screen.queryByRole('button', { name: 'Add authentication provider' })).not.toBeInTheDocument()
  expect(screen.getByRole('switch', { name: 'Storage & Image Transformation' })).not.toBeChecked()
  expect(screen.queryByLabelText('Storage backend')).not.toBeInTheDocument()
})

it('closes the authentication provider picker when Authentication is disabled', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  expect(await screen.findByRole('menuitem', { name: 'Google' })).toBeVisible()

  await user.click(screen.getByRole('switch', { name: 'Authentication' }))
  await user.click(screen.getByRole('switch', { name: 'Authentication' }))

  expect(screen.getByRole('button', { name: 'Add authentication provider' })).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('menuitem', { name: 'Google' })).not.toBeInTheDocument()
})

it('shows Custom SMTP configuration fields after its switch is enabled', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(screen.queryByLabelText('Host')).not.toBeInTheDocument()
  await user.click(screen.getByRole('switch', { name: 'Custom SMTP' }))
  expect(await screen.findByLabelText('Host')).toBeVisible()
  expect(screen.getByLabelText('Port')).toBeVisible()
  expect(screen.getByLabelText('Username')).toBeVisible()
  expect(screen.getByLabelText('Password')).toBeVisible()
  expect(screen.getByLabelText('Sender email')).toBeVisible()
  expect(screen.getByLabelText('Sender name')).toBeVisible()
})

it('shows the identity fields in the Basic step', () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(screen.getByLabelText('Server name')).toBeVisible()
  expect(screen.getByLabelText('Server slug')).toBeVisible()
  expect(screen.getByLabelText('Site URL hostname')).toBeVisible()
  expect(screen.getByLabelText('Studio username')).toBeVisible()
  expect(screen.getByLabelText('Studio password')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Runtime settings' })).toBeVisible()
  expect(screen.queryByLabelText('Pinned Supabase version')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Server URL')).not.toBeInTheDocument()
})

it('uses the InputGroup control slot for the Site URL focus state', () => {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  expect(screen.getByLabelText('Site URL hostname')).toHaveAttribute('data-slot', 'input-group-control')
})

it('shows available project identity feedback before enabling Continue', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  await waitForIdentityAvailability()
  expect(screen.getAllByRole('status')).toHaveLength(2)
  expect(screen.getAllByRole('status')[0]).toHaveAttribute('aria-live', 'polite')
})

it('associates identity validation errors with their inputs', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  const name = screen.getByLabelText('Server name')
  const slug = screen.getByLabelText('Server slug')
  await user.type(name, 'x'.repeat(81))
  await user.clear(slug)
  await user.type(slug, 'Invalid slug')

  await waitFor(() => {
    expect(name).toHaveAttribute('aria-invalid', 'true')
    expect(slug).toHaveAttribute('aria-invalid', 'true')
  })
  expect(name).toHaveAttribute('aria-describedby', 'name-form-item-message')
  expect(slug).toHaveAttribute('aria-describedby', 'slug-form-item-message')
  expect(document.getElementById('name-form-item-message')).toBeVisible()
  expect(document.getElementById('slug-form-item-message')).toBeVisible()
})

it('blocks a duplicate project identity before progression', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => projectListResponse([{ name: 'Production API', slug: 'production-api' }])))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  expect(await screen.findByText('A server named “Production API” already exists')).toBeVisible()
  expect(screen.getByText('The slug “production-api” is already in use')).toBeVisible()
  expect(screen.getByLabelText('Server name')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByLabelText('Server slug')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

it('shows a retryable availability check failure instead of a duplicate', async () => {
  let attempts = 0
  vi.stubGlobal('fetch', vi.fn(async () => {
    attempts += 1
    if (attempts === 1) return new Response(JSON.stringify({ error: { message: 'Servers unavailable' } }), { status: 503, headers: { 'Content-Type': 'application/json' } })
    return projectListResponse()
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  expect(await screen.findByText('Could not check server name availability. Try again.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  await user.click(screen.getAllByRole('button', { name: 'Retry' })[0])
  await waitForIdentityAvailability()
})

it('installs Lightweight after name and base site URL', async () => {
  let createBody: Record<string, unknown> = {}
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    createBody = JSON.parse(String(init?.body)) as Record<string, unknown>
    return new Response(JSON.stringify({ projectId: 'project-1', operationId: 'operation-1' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}>
      <MemoryRouter><NewProjectPage /></MemoryRouter>
    </QueryClientProvider>,
  )

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  expect(screen.getByLabelText('Server slug')).toHaveValue('production-api')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByText('Lightweight')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  expect(createBody.preset).toBe('LIGHTWEIGHT')
  expect(screen.getByRole('heading', { level: 1, name: 'Installing Production API' })).toBeVisible()
})

it('disables the installation action and announces progress while creation is pending', async () => {
  let resolveCreate: ((response: Response) => void) | undefined
  vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return Promise.resolve(projectListResponse())
    return new Promise<Response>((resolve) => { resolveCreate = resolve })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  const action = await screen.findByRole('button', { name: /Creating operation/ })
  expect(action).toBeDisabled()
  expect(action).toHaveAttribute('aria-busy', 'true')
  expect(screen.getByRole('status', { name: 'Creating operation' })).toBeVisible()
  expect(action.querySelector('[data-slot="spinner"][data-icon="inline-start"]')).toBeInTheDocument()

  resolveCreate?.(new Response(JSON.stringify({ projectId: 'project-pending', operationId: 'operation-pending' }), { status: 202, headers: { 'Content-Type': 'application/json' } }))
  expect(await screen.findByRole('heading', { level: 1, name: 'Installing Production API' })).toBeVisible()
})

it('prefixes the Basic-step Site URL with HTTPS before submitting', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init?.body))
    return new Response(JSON.stringify({ projectId: 'project-https', operationId: 'operation-https' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  expect(screen.getByText('https://')).toBeVisible()
  await user.type(screen.getByLabelText('Site URL hostname'), 'app.example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(body?.configuration.general.siteUrl).toBe('https://app.example.com'))
})

it('collects Studio username and password during project creation', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init?.body))
    return new Response(JSON.stringify({ projectId: 'project-studio', operationId: 'operation-studio' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')

  expect(screen.getByLabelText('Studio username')).toBeVisible()
  expect(screen.getByLabelText('Studio password')).toHaveAttribute('type', 'password')
  await user.clear(screen.getByLabelText('Studio username'))
  await user.type(screen.getByLabelText('Studio username'), 'admin')
  await user.type(screen.getByLabelText('Studio password'), 'strong-password')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))
  await waitFor(() => expect(body?.configuration.general).toMatchObject({ studioUsername: 'admin', studioPassword: { action: 'replace', value: 'strong-password' } }))
})

it('does not expose a Review shortcut when required Basic fields are invalid', async () => {
  const fetchSpy = vi.fn()
  vi.stubGlobal('fetch', fetchSpy)
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  expect(screen.queryByRole('button', { name: 'Review' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  expect(fetchSpy).toHaveBeenCalledWith('/api/projects', expect.anything())
})

it('keeps infrastructure settings collapsed until their section is opened', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(screen.getByRole('button', { name: 'Database and Realtime settings' })).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByRole('button', { name: 'Connection pooler settings' })).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByRole('button', { name: 'Gateway and network settings' })).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByLabelText('HTTPS mode')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Gateway and network settings' }))
  expect(screen.getByRole('button', { name: 'Gateway and network settings' })).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByLabelText('HTTPS mode')).toBeVisible()
})

it('keeps a Realtime validation error on review and focuses its field', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Database and Realtime settings' }))
  const realtimeMaxConnections = screen.getAllByRole('spinbutton', { name: 'Max connections' })[1]
  await user.clear(realtimeMaxConnections)
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  expect(screen.getByText('Step 4 of 4 · Review & install')).toBeVisible()
  expect(realtimeMaxConnections).toHaveAttribute('aria-invalid', 'true')
  await waitFor(() => expect(realtimeMaxConnections).toHaveFocus())
})

it('reopens the Realtime section before focusing an invalid field during installation', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  const section = screen.getByRole('button', { name: 'Database and Realtime settings' })
  await user.click(section)
  await user.clear(screen.getAllByRole('spinbutton', { name: 'Max connections' })[1])
  await user.click(section)
  expect(section).toHaveAttribute('aria-expanded', 'false')
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  expect(section).toHaveAttribute('aria-expanded', 'true')
  const realtimeMaxConnections = screen.getAllByRole('spinbutton', { name: 'Max connections' })[1]
  expect(realtimeMaxConnections).toHaveAttribute('aria-invalid', 'true')
  await waitFor(() => expect(realtimeMaxConnections).toHaveFocus())
})

it('opens and focuses infrastructure settings from the review summary edit action', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  const section = screen.getByRole('button', { name: 'Database and Realtime settings' })
  expect(section).toHaveAttribute('aria-expanded', 'false')

  await user.click(screen.getByRole('button', { name: 'Edit infrastructure' }))

  expect(section).toHaveAttribute('aria-expanded', 'true')
  await waitFor(() => expect(section).toHaveFocus())
})

it('redacts dynamic OAuth secrets in the final review summary', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'google-client')
  await user.type(screen.getByLabelText('Client secret'), 'google-secret')
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(screen.getByText('Google (Configured)')).toBeVisible()
  expect(screen.queryByText('google-secret')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Edit security and integrations' })).toBeVisible()
})

it('returns to project details and refreshes availability after a create conflict', async () => {
  let createAttempted = false
  const fetchSpy = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    createAttempted = true
    return new Response(JSON.stringify({ error: { code: 'PROJECT_EXISTS', message: 'This server already exists' } }), { status: 409, headers: { 'Content-Type': 'application/json' } })
  })
  vi.stubGlobal('fetch', fetchSpy)
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(createAttempted).toBe(true))
  expect(await screen.findByText('This server already exists')).toBeVisible()
  expect(screen.getByText('Step 1 of 4 · Server details')).toBeVisible()
  await waitFor(() => expect(fetchSpy.mock.calls.filter(([, init]) => !init?.method || init.method === 'GET').length).toBeGreaterThan(1))
})

it('submits the normalized Standard service aggregate with a dynamically added OAuth provider', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init.body))
    return new Response(JSON.stringify({ projectId: 'project-standard-oauth', operationId: 'operation-standard-oauth' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Standard' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'google-client')
  await user.type(screen.getByLabelText('Client secret'), 'google-secret')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ preset: 'CUSTOM', configuration: { services: { storage: true, functions: true, supavisor: true }, auth: { oauth: { google: { enabled: true, clientId: 'google-client', secret: { action: 'replace', value: 'google-secret' } } } } } })
  expect(body?.configuration.network).not.toHaveProperty('certificate')
})

it('posts the complete aggregate after navigating every wizard step', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { if (!init?.method || init.method === 'GET') return projectListResponse(); body = JSON.parse(String(init?.body)); return new Response(JSON.stringify({ projectId: 'project-2', operationId: 'operation-2' }), { status: 202, headers: { 'Content-Type': 'application/json' } }) }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  for (let index = 0; index < 3; index += 1) await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))
  await waitFor(() => expect(body?.configuration).toBeDefined())
  expect(body?.supabaseVersion).toBeUndefined()
  expect(body?.configuration.auth.email.secureEmailChange).toBe(false)
  expect(body?.configuration.realtime).toEqual({ maxConnections: 100, databasePoolSize: 5, logLevel: 'info' })
  expect(body?.configuration.storage.forcePathStyle).toBe(false)
  expect(body?.configuration.pooler.maxClientConnections).toBe(100)
  expect(body?.configuration.pooler.transactionPort).toBe(0)
  expect(body?.configuration.pooler.sessionPort).toBe(0)
  expect(body?.configuration.services.database).toBe(true)
  expect(body).not.toHaveProperty('domain')
  expect(body).not.toHaveProperty('siteUrl')
  expect(body).not.toHaveProperty('services')
})

it('uses Standard aggregate controls and closes Direct DB through Custom without a hard-coded port', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { if (!init?.method || init.method === 'GET') return projectListResponse(); body = JSON.parse(String(init?.body)); return new Response(JSON.stringify({ projectId: 'project-3', operationId: 'operation-3' }), { status: 202, headers: { 'Content-Type': 'application/json' } }) }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Standard' }))
  expect(screen.getByRole('switch', { name: 'Edge Functions' })).toBeChecked()
  expect(screen.getByRole('switch', { name: 'Storage' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Database and Realtime settings' }))
  await user.click(screen.getByRole('switch', { name: 'Direct PostgreSQL port' }))
  expect(screen.getByRole('switch', { name: 'Direct PostgreSQL port' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Install server' }))
  await waitFor(() => expect(body?.configuration).toBeDefined())
  expect(body?.preset).toBe('CUSTOM')
  expect(body?.configuration.services.functions).toBe(true)
  expect(body?.configuration.services.directDb).toBe(true)
  expect(body?.configuration.database.directPort).toBe(true)
  expect(body?.configuration.database.directPortNumber).toBe(0)
  expect(body?.configuration.network.directDatabasePort).toBe(0)
})

it('restores the full dependency closure when a feature is enabled again', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'Authentication' }))
  await user.click(screen.getByRole('switch', { name: 'PostgREST' }))
  await user.click(screen.getByRole('switch', { name: 'Supabase Studio' }))
  await user.click(screen.getByRole('switch', { name: 'API Gateway' }))
  expect(screen.getByRole('switch', { name: 'API Gateway' })).not.toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Edge Functions' }))
  expect(screen.getByRole('switch', { name: /^API Gateway/ })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^PostgreSQL/ })).toBeChecked()
})

it('enabling Storage or Image Transformation atomically restores database, REST, and gateway', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'PostgREST' }))
  expect(screen.getByRole('switch', { name: 'PostgREST' })).not.toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Storage' }))
  expect(screen.getByRole('switch', { name: 'Storage' })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^PostgREST/ })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^API Gateway/ })).toBeChecked()
  expect(screen.getByRole('switch', { name: /^PostgreSQL/ })).toBeChecked()
  await user.click(screen.getByRole('switch', { name: 'Image Transformation' }))
  expect(screen.getByRole('switch', { name: 'Image Transformation' })).toBeChecked()
})

it('renders nested OAuth secret value errors at the secret control', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'client')
  await user.type(screen.getByLabelText('Client secret'), ' ')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByText('A replacement value is required')).toBeVisible()
})

it('submits dynamically added OAuth credentials through the existing aggregate configuration', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init.body))
    return new Response(JSON.stringify({ projectId: 'project-oauth', operationId: 'operation-oauth' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Google' }))
  await user.type(screen.getByLabelText('Client ID'), 'google-client')
  await user.type(screen.getByLabelText('Client secret'), 'google-secret')
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(body?.configuration.auth.oauth.google).toMatchObject({ enabled: true, clientId: 'google-client', secret: { action: 'replace', value: 'google-secret' } }))
})

it('clears incomplete dynamically added OAuth configuration when Authentication is disabled', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init.body))
    return new Response(JSON.stringify({ projectId: 'project-auth-disabled', operationId: 'operation-auth-disabled' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Google' }))
  await user.click(screen.getByRole('switch', { name: 'Authentication' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByText('Step 4 of 4 · Review & install')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(body?.configuration.auth.oauth).toEqual({}))
})

it('clears Custom SMTP credentials from the DTO when Authentication is disabled', async () => {
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init.body))
    return new Response(JSON.stringify({ projectId: 'project-smtp-disabled', operationId: 'operation-smtp-disabled' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'Custom SMTP' }))
  await user.type(screen.getByLabelText('Host'), 'smtp.example.com')
  await user.type(screen.getByLabelText('Username'), 'mailer')
  await user.type(screen.getByLabelText('Password'), 'smtp-secret')
  await user.type(screen.getByLabelText('Sender email'), 'hello@example.com')
  await user.type(screen.getByLabelText('Sender name'), 'Example')
  await user.click(screen.getByRole('switch', { name: 'Authentication' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(body?.configuration.auth.smtp).toEqual({ enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' }))
})

it('clears Custom SMTP credentials when its module is disabled while Authentication remains enabled', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let body: Record<string, any> | undefined
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init.body))
    return new Response(JSON.stringify({ projectId: 'project-smtp-module-disabled', operationId: 'operation-smtp-module-disabled' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)

  await user.type(screen.getByLabelText('Server name'), 'Production API')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('switch', { name: 'Custom SMTP' }))
  await user.type(screen.getByLabelText('Host'), 'smtp.example.com')
  await user.type(screen.getByLabelText('Username'), 'mailer')
  await user.type(screen.getByLabelText('Password'), 'smtp-secret')
  await user.type(screen.getByLabelText('Sender email'), 'hello@example.com')
  await user.type(screen.getByLabelText('Sender name'), 'Example')
  await user.click(screen.getByRole('switch', { name: 'Custom SMTP' }))
  expect(screen.getByRole('switch', { name: 'Authentication' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))

  await waitFor(() => expect(body?.configuration.auth.smtp).toEqual({ enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' }, senderEmail: '', senderName: '' }))
})

it('forces R2 path-style and submits the upload limit in bytes', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let body: any
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    body = JSON.parse(String(init.body))
    return new Response(JSON.stringify({ projectId: 'r2', operationId: 'op-r2' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'R2 server')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Standard' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('combobox', { name: 'Storage backend' }))
  await user.click(await screen.findByText('Cloudflare R2'))
  expect(screen.queryByText('Force path style')).not.toBeInTheDocument()
  await user.type(screen.getByLabelText('Bucket'), 'objects')
  await user.type(screen.getByLabelText('Account ID'), 'abcdef0123456789abcdef0123456789')
  await user.type(screen.getByLabelText('Access key ID'), 'access')
  await user.type(screen.getByLabelText('Secret access key'), 'secret')
  fireEvent.change(screen.getByLabelText('Upload limit (MiB)'), { target: { value: '512' } })
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Install server' }))
  await waitFor(() => expect(body?.configuration.storage).toMatchObject({ backend: 'r2', forcePathStyle: true, uploadFileSizeLimit: 512 * 1024 * 1024 }))
})

it('blocks an invalid R2 upload limit instead of preserving the previous bytes', async () => {
  window.PointerEvent = class extends window.MouseEvent {} as typeof PointerEvent
  let submitted = false
  vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (!init?.method || init.method === 'GET') return projectListResponse()
    submitted = true
    return new Response(JSON.stringify({ projectId: 'r2-invalid', operationId: 'op-r2-invalid' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }))
  const user = userEvent.setup()
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })}><MemoryRouter><NewProjectPage /></MemoryRouter></QueryClientProvider>)
  await user.type(screen.getByLabelText('Server name'), 'R2 invalid')
  await user.type(screen.getByLabelText('Site URL hostname'), 'example.com')
  await waitForIdentityAvailability()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('button', { name: 'Standard' }))
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  await user.click(screen.getByRole('combobox', { name: 'Storage backend' }))
  await user.keyboard('{ArrowDown}{ArrowDown}{Enter}')
  const limit = screen.getByLabelText('Upload limit (MiB)')
  fireEvent.change(limit, { target: { value: '5121' } })
  expect(screen.getByText(/between 1 and 5120 MiB/i)).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Continue' }))
  expect(screen.getByText('Step 3 of 4 · Security & integrations')).toBeVisible()
  expect(submitted).toBe(false)
})
