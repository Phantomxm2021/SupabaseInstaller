import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { EmailTemplateEditorPage } from './EmailTemplateEditorPage'
import type { AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import type { Services } from '../../api/types'
import { defaultMailerConfiguration } from '../projects/projectSchema'

const defaults = defaultMailerConfiguration()
const context = { projectId: 'bee', revision: 3, general: { domain: 'bee.example.test', siteUrl: 'https://bee.example.test', supabaseVersion: '2.0.0' }, services: { auth: true } as Services, requestSave: vi.fn(), auth: { mailer: { ...defaults, templates: { ...defaults.templates, confirmation: { subject: 'Confirm your email address', templateUrl: 'https://templates.example.test/confirmation.html' } } } } } as unknown as AuthenticationWorkspaceContext

describe('EmailTemplateEditorPage', () => {
  it('edits only the GoTrue-supported subject and template URL fields', async () => {
    const user = userEvent.setup(); const requestSave = vi.fn()
    render(<MemoryRouter><EmailTemplateEditorPage context={{ ...context, requestSave }} templateKey="confirm-signup" /></MemoryRouter>)
    expect(screen.queryByLabelText('HTML body')).not.toBeInTheDocument()
    await user.clear(screen.getByLabelText('Template URL')); await user.type(screen.getByLabelText('Template URL'), 'https://cdn.example.test/confirm.html')
    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(requestSave).toHaveBeenCalledWith(expect.objectContaining({ value: expect.objectContaining({ mailer: expect.objectContaining({ templates: expect.objectContaining({ confirmation: expect.objectContaining({ templateUrl: 'https://cdn.example.test/confirm.html' }) }) }) }) }))
  })

  it('previews configured URL templates in a sandbox and resets to GoTrue defaults', async () => {
    const user = userEvent.setup()
    render(<MemoryRouter><EmailTemplateEditorPage context={context} templateKey="confirm-signup" /></MemoryRouter>)
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    expect(screen.getByTitle('Email template preview')).toHaveAttribute('sandbox')
    expect(screen.getByTitle('Email template preview')).toHaveAttribute('src', 'https://templates.example.test/confirmation.html')
    await user.click(screen.getByRole('button', { name: 'Source' })); await user.click(screen.getByRole('button', { name: 'Reset template' }))
    expect(screen.getByLabelText('Template URL')).toHaveValue('')
  })

  it('does not save notification configuration before an explicit edit and save', () => {
    const requestSave = vi.fn()
    render(<MemoryRouter><EmailTemplateEditorPage context={{ ...context, requestSave }} templateKey="password-changed" /></MemoryRouter>)
    expect(screen.getByRole('switch', { name: 'Enable notification' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
    expect(requestSave).not.toHaveBeenCalled()
  })

})
