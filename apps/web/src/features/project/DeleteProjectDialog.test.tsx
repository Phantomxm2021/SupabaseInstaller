import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeleteProjectDialog } from './DeleteProjectDialog'

it('requires the exact project name before deleting data', async () => {
  const user = userEvent.setup()
  render(<DeleteProjectDialog project={{ id: 'bee', name: 'Bee' }} open onClose={() => {}} onDelete={() => {}} />)
  vi.stubGlobal('PointerEvent', MouseEvent)
  expect(screen.getByRole('radiogroup', { name: 'Delete mode' })).toBeInTheDocument()
  await user.click(screen.getByRole('radio', { name: 'Delete runtime and data' }))
  await user.type(screen.getByLabelText('Type Bee to confirm'), 'bee')
  expect(screen.getByRole('button', { name: 'Delete permanently' })).toBeDisabled()
  await user.clear(screen.getByLabelText('Type Bee to confirm'))
  await user.type(screen.getByLabelText('Type Bee to confirm'), 'Bee')
  expect(screen.getByRole('button', { name: 'Delete permanently' })).toBeEnabled()
})

it('uses accessible shadcn radio and field primitives for destructive choices', async () => {
  const user = userEvent.setup()
  vi.stubGlobal('PointerEvent', MouseEvent)
  render(<DeleteProjectDialog project={{ id: 'bee', name: 'Bee' }} open onClose={() => {}} onDelete={() => {}} />)
  expect(screen.getByRole('radiogroup', { name: 'Delete mode' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'Delete runtime only' })).toBeChecked()
  await user.click(screen.getByRole('radio', { name: 'Delete runtime and data' }))
  const confirmation = screen.getByRole('textbox', { name: 'Type Bee to confirm' })
  expect(confirmation).toHaveAttribute('data-slot', 'input')
  expect(screen.getByText('Type Bee to confirm')).toBeVisible()
})
