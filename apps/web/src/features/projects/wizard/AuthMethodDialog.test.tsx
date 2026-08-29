import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { AuthMethodDialog } from './AuthMethodDialog'

it('offers only unconfigured third-party OAuth providers from its accessible picker', async () => {
  const user = userEvent.setup()
  const onSelect = vi.fn()
  const onOpenChange = vi.fn()
  render(<AuthMethodDialog open onOpenChange={onOpenChange} addedOAuth={[]} onSelect={onSelect} />)

  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  expect(screen.getByRole('menuitem', { name: 'Google' })).toBeVisible()
  expect(screen.queryByRole('menuitem', { name: /Email password|Magic Link|Custom SMTP|Phone Auth|Anonymous sign-in/ })).not.toBeInTheDocument()

  await user.click(screen.getByRole('menuitem', { name: 'Google' }))
  expect(onSelect).toHaveBeenCalledWith({ kind: 'oauth', provider: 'google' })
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

it('omits OAuth providers that have already been added', async () => {
  const user = userEvent.setup()
  render(<AuthMethodDialog open onOpenChange={vi.fn()} addedOAuth={['google']} onSelect={vi.fn()} />)

  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  expect(screen.queryByRole('menuitem', { name: 'Google' })).not.toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'GitHub' })).toBeVisible()
})

it('opens when its trigger is activated', async () => {
  const user = userEvent.setup()
  render(<PickerHarness />)

  await user.click(screen.getByRole('button', { name: 'Add authentication provider' }))
  expect(await screen.findByRole('menuitem', { name: 'Google' })).toBeVisible()
})

function PickerHarness() {
  const [open, setOpen] = useState(false)
  return <AuthMethodDialog open={open} onOpenChange={setOpen} addedOAuth={[]} onSelect={vi.fn()} />
}
