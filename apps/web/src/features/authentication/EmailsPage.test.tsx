import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { EmailsPage } from './EmailsPage'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import type { Services } from '../../api/types'

const smtp = { enabled: false, host: '', port: 587, username: '', passwordSet: false, password: { action: '' as const }, senderEmail: '', senderName: '' }
const context: AuthenticationWorkspaceContext = {
  projectId: 'bee', revision: 1,
  general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' },
  services: { auth: true } as Services,
  auth: { enabled: true, jwtExpiry: 3600, disableSignup: false, email: { enabled: true, allowSignup: true, confirmEmail: false, secureEmailChange: false, doubleConfirmChanges: false }, phone: { enabled: false, provider: '', secretSet: false, secret: { action: '' }, fields: {} }, anonymousSignIn: false, redirectUrls: [], oauth: {}, smtp, mailer: { templates: { confirmation: { subject: 'Confirm your signup', body: '<p>{{ .ConfirmationURL }}</p>' }, invite: { subject: 'Invitation', body: '<p>{{ .ConfirmationURL }}</p>' }, magicLink: { subject: 'Magic link', body: '<p>{{ .ConfirmationURL }}</p>' }, emailChange: { subject: 'Confirm email change', body: '<p>{{ .ConfirmationURL }}</p>' }, recovery: { subject: 'Reset password', body: '<p>{{ .ConfirmationURL }}</p>' }, reauthentication: { subject: 'Confirm reauthentication', body: '<p>{{ .Token }}</p>' } }, notifications: { passwordChanged: { enabled: false, template: { subject: 'Password changed', body: '<p>{{ .Email }}</p>' } }, emailChanged: { enabled: false, template: { subject: 'Email changed', body: '<p>{{ .Email }}</p>' } }, phoneChanged: { enabled: false, template: { subject: 'Phone changed', body: '<p>{{ .Email }}</p>' } }, identityLinked: { enabled: false, template: { subject: 'Identity linked', body: '<p>{{ .Email }}</p>' } }, identityUnlinked: { enabled: false, template: { subject: 'Identity unlinked', body: '<p>{{ .Email }}</p>' } }, mfaFactorEnrolled: { enabled: false, template: { subject: 'MFA added', body: '<p>{{ .Email }}</p>' } }, mfaFactorUnenrolled: { enabled: false, template: { subject: 'MFA removed', body: '<p>{{ .Email }}</p>' } } } }, rateLimits: { emailSent: 30, smsSent: 30, tokenRefresh: 150, tokenVerification: 30, anonymousUsers: 30, signupsAndSignins: 30 }, mfa: { totpEnrollEnabled: true, totpVerifyEnabled: true, phoneEnrollEnabled: false, phoneVerifyEnabled: false, maxEnrolledFactors: 10, phoneOtpLength: 6 } },
  requestSave: vi.fn(),
}

describe('EmailsPage', () => {
  it('renders authentication and security template groups with navigable rows', async () => {
    const user = userEvent.setup()
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><EmailsPage context={context} /></MemoryRouter></QueryClientProvider>)
    expect(screen.getByRole('heading', { name: 'Emails' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Templates' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Confirm sign up/i })).toHaveAttribute('href', '/projects/bee/authentication/emails/confirm-signup')
    expect(screen.getByRole('heading', { name: 'Security' })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enable password changed notification' })).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })

  it('shows an SMTP form and queues a dirty SMTP update through the workspace confirmation path', async () => {
    const user = userEvent.setup()
    const requestSave = vi.fn()
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><EmailsPage context={{ ...context, requestSave, auth: { ...context.auth, smtp: { ...smtp, enabled: true } } }} /></MemoryRouter></QueryClientProvider>)
    await user.click(screen.getByRole('tab', { name: 'SMTP Settings' }))
    expect(screen.getByRole('heading', { name: 'SMTP settings' })).toBeInTheDocument()
    await user.type(screen.getByLabelText('Sender email address'), 'no-reply@bee.example.test')
    await user.type(screen.getByLabelText('Sender name'), 'Bee')
    await user.type(screen.getByLabelText('Host'), 'smtp.bee.example.test')
    await user.clear(screen.getByLabelText('Port number'))
    await user.type(screen.getByLabelText('Port number'), '465')
    await user.type(screen.getByLabelText('Username'), 'bee')
    await user.type(screen.getByLabelText('Password'), 'secret')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(requestSave).toHaveBeenCalledTimes(1))
    expect(requestSave.mock.calls[0][0]).toMatchObject({ section: 'smtp', value: { enabled: true, host: 'smtp.bee.example.test', port: 465, senderEmail: 'no-reply@bee.example.test' } })
  })

  it('asks before discarding dirty SMTP fields while changing tabs', async () => {
    const user = userEvent.setup()
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><EmailsPage context={{ ...context, auth: { ...context.auth, smtp: { ...smtp, enabled: true } } }} /></MemoryRouter></QueryClientProvider>)
    await user.click(screen.getByRole('tab', { name: 'SMTP Settings' }))
    await user.type(screen.getByLabelText('Host'), 'smtp.bee.example.test')
    await user.click(screen.getByRole('tab', { name: 'Templates' }))
    expect(screen.getByRole('alertdialog')).toHaveTextContent('Discard SMTP changes?')
    await user.click(screen.getByRole('button', { name: 'Discard changes' }))
    expect(screen.getByRole('link', { name: /Confirm sign up/i })).toBeInTheDocument()
  })

  it('normalizes a legacy configuration without mailer data', () => {
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><EmailsPage context={{ ...context, auth: { ...context.auth, mailer: undefined } }} /></MemoryRouter></QueryClientProvider>)
    expect(screen.getByRole('link', { name: /Confirm sign up/i })).toBeInTheDocument()
  })
})
