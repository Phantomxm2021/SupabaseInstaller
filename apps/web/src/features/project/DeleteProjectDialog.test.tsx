import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeleteProjectDialog } from './DeleteProjectDialog'

it('requires the exact project name before deleting data', async () => {
  const user = userEvent.setup()
  render(<DeleteProjectDialog project={{ id: 'bee', name: 'Bee' }} open onClose={() => {}} onDelete={() => {}} />)
  await user.click(screen.getByLabelText('Delete runtime and data'))
  await user.type(screen.getByLabelText('Type Bee to confirm'), 'bee')
  expect(screen.getByRole('button', { name: 'Delete permanently' })).toBeDisabled()
  await user.clear(screen.getByLabelText('Type Bee to confirm'))
  await user.type(screen.getByLabelText('Type Bee to confirm'), 'Bee')
  expect(screen.getByRole('button', { name: 'Delete permanently' })).toBeEnabled()
})
