import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { AuthMethodDialog } from './AuthMethodDialog'

it('filters a single-column authentication method list and adds an available OAuth provider', async () => {
  const user = userEvent.setup()
  const onSelect = vi.fn()
  render(<AuthMethodDialog open onOpenChange={vi.fn()} addedOAuth={[]} onSelect={onSelect} />)

  expect(screen.getByRole('heading', { name: 'Add authentication method' })).toBeVisible()
  await user.type(screen.getByLabelText('Search authentication methods'), 'git')
  expect(screen.getByRole('button', { name: 'GitHub' })).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Google' })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'GitHub' }))
  expect(onSelect).toHaveBeenCalledWith({ kind: 'oauth', provider: 'github' })
})

it('filters providers and omits OAuth providers that have already been added', async () => {
  const user = userEvent.setup()
  render(<AuthMethodDialog open onOpenChange={vi.fn()} addedOAuth={['google']} onSelect={vi.fn()} />)

  await user.click(screen.getByRole('button', { name: 'OAuth providers' }))
  expect(screen.queryByRole('button', { name: 'Google' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'GitHub' })).toBeVisible()
})
