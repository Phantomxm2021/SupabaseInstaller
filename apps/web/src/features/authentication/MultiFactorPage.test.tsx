import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthConfig, Services } from '../../api/types'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import { MultiFactorPage } from './MultiFactorPage'
import { defaultConfiguration } from '../projects/projectSchema'

const auth = { ...defaultConfiguration().auth, email: { ...defaultConfiguration().auth.email, confirmEmail: true } } as AuthConfig
const context = { projectId: 'bee', revision: 2, auth, general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' }, services: { auth: true } as Services, requestSave: vi.fn() } as AuthenticationWorkspaceContext

describe('MultiFactorPage', () => {
  it('renders the source-style MFA factor controls', () => {
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><MultiFactorPage context={context} /></MemoryRouter></QueryClientProvider>)
    expect(screen.getByRole('heading', { level: 1, name: 'Multi-Factor Authentication (MFA)' })).toBeVisible()
    expect(screen.getByRole('main')).toHaveClass('dashboard-stack')
    expect(screen.getByLabelText('TOTP (App Authenticator)')).toHaveValue('enabled')
    expect(screen.getByLabelText('Maximum number of per-user MFA factors')).toHaveValue(10)
    expect(screen.getByText('SMS MFA')).toBeVisible()
    expect(screen.getByLabelText('Phone')).toHaveValue('disabled')
    expect(screen.getByLabelText('Phone OTP length')).toHaveValue(6)
  })

  it('saves MFA changes as a full Auth configuration', async () => {
    const user = userEvent.setup()
    const requestSave = vi.fn()
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><MultiFactorPage context={{ ...context, requestSave }} /></MemoryRouter></QueryClientProvider>)
    fireEvent.change(screen.getByLabelText('Phone OTP length'), { target: { value: '8', valueAsNumber: 8 } })
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(requestSave).toHaveBeenCalledTimes(1))
    expect(requestSave.mock.calls[0][0]).toMatchObject({ section: 'auth', value: { mfa: { phoneEnrollEnabled: false, phoneOtpLength: 8 } } })
  })
})
