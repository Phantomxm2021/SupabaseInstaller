import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { CopyButton } from './CopyButton'
import { ReadOnlyField } from '@/features/project/configuration/fields'

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('sonner', () => ({ toast }))

it('places a read-only field copy control in the label row above its input', () => {
  render(<ReadOnlyField label="Project URL" value="https://project.example.com" copy />)

  const label = screen.getByText('Project URL')
  const button = screen.getByRole('button', { name: 'Copy Project URL' })
  const input = screen.getByDisplayValue('https://project.example.com')
  expect(label.parentElement).toContainElement(button)
  expect(label.parentElement).not.toContainElement(input)
})

it('uses an icon button and confirms a successful copy visually and with a toast', async () => {
  const user = userEvent.setup()
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })

  render(<CopyButton value="https://project.example.com" label="Project URL" />)

  const button = screen.getByRole('button', { name: 'Copy Project URL' })
  expect(button).toHaveAttribute('title', 'Copy Project URL')
  expect(screen.queryByText('Copy')).not.toBeInTheDocument()

  await user.click(button)

  await waitFor(() => expect(writeText).toHaveBeenCalledWith('https://project.example.com'))
  expect(button).toHaveAccessibleName('Copied Project URL')
  expect(toast.success).toHaveBeenCalledWith('Project URL copied')
})
