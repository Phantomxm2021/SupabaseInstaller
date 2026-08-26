import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ProjectLayout } from './ProjectLayout'

it('keeps project navigation collapsed until a discoverable shadcn trigger opens it', async () => {
  const user = userEvent.setup()
  const { container } = render(<MemoryRouter initialEntries={['/projects/bee/overview']}><Routes><Route path="/projects/:projectId/*" element={<ProjectLayout />}><Route path="overview" element={<div>Project overview</div>} /></Route></Routes></MemoryRouter>)
  const sidebar = container.querySelector('[data-slot="sidebar"]')
  expect(sidebar).toHaveAttribute('data-collapsible', 'offcanvas')
  expect(screen.getByRole('button', { name: 'Open project navigation' })).toBeVisible()
  expect(screen.getByText('Project overview')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Open project navigation' }))
  expect(sidebar).toHaveAttribute('data-state', 'expanded')
  expect(screen.getByRole('link', { name: 'Configuration' })).toBeVisible()
})

it('uses the mobile offcanvas surface without squeezing the project content', async () => {
  const user = userEvent.setup()
  const previousWidth = window.innerWidth
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 640 })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const { container } = render(<MemoryRouter initialEntries={['/projects/bee/overview']}><Routes><Route path="/projects/:projectId/*" element={<ProjectLayout />}><Route path="overview" element={<div>Project overview</div>} /></Route></Routes></MemoryRouter>)
  expect(screen.getByText('Project overview')).toBeVisible()
  expect(container.querySelector('[data-slot="sidebar"][data-mobile="true"]')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open project navigation' }))
  expect(await screen.findByRole('link', { name: 'Configuration' })).toBeVisible()
  expect(document.querySelector('[data-slot="sidebar"][data-mobile="true"]')).toBeVisible()
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: previousWidth })
  vi.unstubAllGlobals()
})
