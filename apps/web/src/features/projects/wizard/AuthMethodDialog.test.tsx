import { useState } from 'react'
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

it('resets the search and category when reopened', async () => {
  const user = userEvent.setup()
  render(<DialogHarness />)

  await user.click(screen.getByRole('button', { name: 'Open dialog' }))
  await user.click(screen.getByRole('button', { name: 'OAuth providers' }))
  await user.type(screen.getByLabelText('Search authentication methods'), 'git')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await user.click(screen.getByRole('button', { name: 'Open dialog' }))

  expect(screen.getByLabelText('Search authentication methods')).toHaveValue('')
  expect(screen.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true')
})

function DialogHarness() {
  const [open, setOpen] = useState(false)
  return <><button type="button" onClick={() => setOpen(true)}>Open dialog</button><AuthMethodDialog open={open} onOpenChange={setOpen} addedOAuth={[]} onSelect={vi.fn()} /></>
}
