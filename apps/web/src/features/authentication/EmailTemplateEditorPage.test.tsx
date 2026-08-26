import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { EmailTemplateEditorPage } from './EmailTemplateEditorPage'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import type { Services } from '../../api/types'

const context = { projectId: 'bee', revision: 3, general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' }, services: { auth: true } as Services, requestSave: vi.fn(), auth: { mailer: { templates: { confirmation: { subject: 'Confirm your email address', body: '<p>{{ .ConfirmationURL }}</p>' } }, notifications: {} } } } as unknown as AuthenticationWorkspaceContext

describe('EmailTemplateEditorPage', () => {
  it('inserts documented template variables at the focused subject or body caret', async () => {
    const user = userEvent.setup()
    render(<MemoryRouter><EmailTemplateEditorPage context={context} templateKey="confirm-signup" /></MemoryRouter>)
    await user.click(screen.getByLabelText('Subject'))
    await user.click(screen.getByRole('button', { name: '{{ .Token }}' }))
    expect(screen.getByLabelText('Subject')).toHaveValue('Confirm your email address{{ .Token }}')
    await user.click(screen.getByLabelText('HTML body'))
    await user.click(screen.getByRole('button', { name: '{{ .Token }}' }))
    expect(screen.getByLabelText('HTML body')).toHaveValue('<p>{{ .ConfirmationURL }}</p>{{ .Token }}')
    await user.clear(screen.getByLabelText('Subject'))
    await user.type(screen.getByLabelText('Subject'), 'Please {{ .NotSupported }}')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(screen.getByText(/template action|Unsupported template variable/)).toBeInTheDocument()
    expect(context.requestSave).not.toHaveBeenCalled()
  })

  it('resets a template to the Manager default and keeps preview sandboxed', async () => {
    const user = userEvent.setup()
    render(<MemoryRouter><EmailTemplateEditorPage context={context} templateKey="confirm-signup" /></MemoryRouter>)
    await user.clear(screen.getByLabelText('Subject'))
    await user.type(screen.getByLabelText('Subject'), 'Custom subject')
    await user.click(screen.getByRole('button', { name: 'Reset template' }))
    expect(screen.getByLabelText('Subject')).toHaveValue('Confirm your signup')
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    expect(screen.getByTitle('Email template preview')).toHaveAttribute('sandbox')
  })

  it('keeps a notification toggle local until the explicit save action', async () => {
    const user = userEvent.setup()
    const requestSave = vi.fn()
    render(<MemoryRouter><EmailTemplateEditorPage context={{ ...context, requestSave }} templateKey="password-changed" /></MemoryRouter>)
    expect(requestSave).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })
})
