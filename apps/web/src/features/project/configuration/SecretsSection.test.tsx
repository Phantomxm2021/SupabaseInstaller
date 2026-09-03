import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SecretsSection } from './SecretsSection'

function renderSecrets() {
  return render(
    <SecretsSection projectId="project-1" projectName="Acme Project" projectUrl="https://acme.example.com" />,
  )
}

afterEach(() => vi.restoreAllMocks())

it('requires a password before submitting an auth-key migration', async () => {
  const user = userEvent.setup()
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ projectId: 'project-1', operationId: 'op-1' }), { status: 202 }))
  vi.stubGlobal('fetch', fetchMock)
  renderSecrets()

  await user.click(screen.getByRole('button', { name: /reveal api keys/i }))
  expect(screen.getByRole('button', { name: /migrate auth keys/i })).toBeDisabled()
  await user.type(screen.getByLabelText(/administrator password/i), 'correct horse')
  await user.click(screen.getByRole('button', { name: /migrate auth keys/i }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
  expect(JSON.parse(String(init.body))).toEqual({ password: 'correct horse' })
})

it('reveals only public and opaque API keys and never renders private JWT material', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ kind: 'publishable-api-key', value: 'sb_publishable_test' }), { status: 200 })))
  renderSecrets()
  await user.click(screen.getByRole('button', { name: /reveal api keys/i }))
  expect(screen.getByRole('button', { name: /reveal publishable api key/i })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /reveal secret api key/i })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /reveal jwt keys|reveal jwt jwks|reveal asymmetric/i })).not.toBeInTheDocument()
})

it('warns about opaque rotation and gates signing replacement on password and exact project name', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ projectId: 'project-1', operationId: 'op-1' }), { status: 202 })))
  renderSecrets()
  await user.click(screen.getByRole('button', { name: /reveal api keys/i }))

  expect(screen.getByText(/opaque api key rotation preserves signing material/i)).toBeInTheDocument()
  const signing = screen.getByRole('button', { name: /rotate signing keys/i })
  expect(signing).toBeDisabled()
  await user.type(screen.getByLabelText(/administrator password/i), 'secret')
  expect(signing).toBeDisabled()
  await user.type(screen.getByLabelText(/type the project name/i), 'Acme Project')
  expect(signing).toBeEnabled()
  await user.click(signing)
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled())
  const [, init] = vi.mocked(fetch).mock.calls.at(-1) as unknown as [string, RequestInit]
  expect(JSON.parse(String(init.body))).toEqual({ password: 'secret', confirmProjectName: 'Acme Project' })
})
