import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { FunctionsConfig } from '../../../api/types'
import { FunctionsSection } from './FunctionsSection'

const configured = (): FunctionsConfig => ({
  defaultJwtVerification: true,
  variables: [{ name: 'EXISTING_SECRET', valueSet: true, value: { action: '' } }],
})

it('renders Function environment variables as a compact semantic list', () => {
  render(<FunctionsSection initial={configured()} revision={1} enabled onSave={vi.fn()} />)

  expect(screen.getByRole('columnheader', { name: 'Name' })).toBeVisible()
  expect(screen.getByRole('columnheader', { name: 'Value' })).toBeVisible()
  expect(screen.getByRole('columnheader', { name: 'Status' })).toBeVisible()
  expect(screen.getByText('EXISTING_SECRET')).toBeVisible()
  expect(screen.getByText('Configured')).toBeVisible()
})

it('adds a Function environment variable as a replacement command', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<FunctionsSection initial={{ defaultJwtVerification: true, variables: [] }} revision={1} enabled onSave={onSave} />)

  await user.click(screen.getByRole('button', { name: 'Add variable' }))
  await user.type(screen.getByRole('textbox', { name: 'Variable name' }), 'NEW_SECRET')
  await user.type(screen.getByLabelText('Value for NEW_SECRET'), 'new-value')
  await user.click(screen.getByRole('button', { name: 'Save Functions' }))

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
  expect(onSave.mock.calls[0][0].value.variables).toEqual([
    { name: 'NEW_SECRET', valueSet: true, value: { action: 'replace', value: 'new-value' } },
  ])
})

it('marks a configured variable for removal and allows undo before save', async () => {
  const user = userEvent.setup()
  const onSave = vi.fn()
  render(<FunctionsSection initial={configured()} revision={1} enabled onSave={onSave} />)

  await user.click(screen.getByRole('button', { name: 'Remove variable EXISTING_SECRET' }))
  expect(screen.getByText('Pending removal')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Undo removal EXISTING_SECRET' }))
  expect(screen.getByText('Configured')).toBeVisible()

  await user.click(screen.getByRole('button', { name: 'Remove variable EXISTING_SECRET' }))
  await user.click(screen.getByRole('button', { name: 'Save Functions' }))

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1))
  expect(onSave.mock.calls[0][0].value.variables[0]).toEqual({
    name: 'EXISTING_SECRET',
    valueSet: true,
    value: { action: 'remove' },
  })
})
