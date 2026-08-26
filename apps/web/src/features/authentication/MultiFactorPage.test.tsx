import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { AuthConfig, Services } from '../../api/types'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import { MultiFactorPage } from './MultiFactorPage'

const auth = { enabled: true, jwtExpiry: 3600, disableSignup: false, email: { enabled: true, allowSignup: true, confirmEmail: true, secureEmailChange: false, doubleConfirmChanges: false }, phone: { enabled: false, provider: '', secretSet: false, secret: { action: '' as const }, fields: {} }, anonymousSignIn: false, redirectUrls: [], oauth: {}, smtp: { enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' as const }, senderEmail: '', senderName: '' }, rateLimits: { emailSent: 30, smsSent: 30, tokenRefresh: 150, tokenVerification: 30, anonymousUsers: 30, signupsAndSignins: 30 }, mfa: { totpEnrollEnabled: true, totpVerifyEnabled: true, phoneEnrollEnabled: false, phoneVerifyEnabled: false, maxEnrolledFactors: 10, phoneOtpLength: 6 } } as AuthConfig
const context = { projectId: 'bee', revision: 2, auth, general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' }, services: { auth: true } as Services, requestSave: vi.fn() } as AuthenticationWorkspaceContext

describe('MultiFactorPage', () => {
  it('renders TOTP and phone MFA controls with informative descriptions', () => {
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><MultiFactorPage context={context} /></MemoryRouter></QueryClientProvider>)
    expect(screen.getByRole('heading', { level: 1, name: 'Multi-Factor Authentication (MFA)' })).toBeVisible()
    expect(screen.getByRole('switch', { name: 'TOTP (App Authenticator) enrollment' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByLabelText('Maximum number of per-user MFA factors')).toHaveValue(10)
    expect(screen.getByText('SMS MFA')).toBeVisible()
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
