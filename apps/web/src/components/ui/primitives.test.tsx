import { render, screen } from "@testing-library/react"

import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "./command"
import { Badge } from "./badge"
import { Button } from "./button"
import { Card } from "./card"
import { Input } from "./input"
import { Label } from "./label"
import { Spinner } from "./spinner"
import { Table } from "./table"
import { Textarea } from "./textarea"

it("exposes searchable command items and an accessible saving spinner", () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  HTMLElement.prototype.scrollIntoView = vi.fn()

  render(
    <>
      <Command>
        <CommandInput placeholder="Search repositories" />
        <CommandList>
          <CommandEmpty>No repositories found.</CommandEmpty>
          <CommandItem>GitHub</CommandItem>
        </CommandList>
      </Command>
      <Spinner aria-label="Saving" />
    </>,
  )

  expect(screen.getByPlaceholderText("Search repositories")).toBeInTheDocument()
  expect(screen.getByText("GitHub")).toBeInTheDocument()
  expect(screen.getByLabelText("Saving")).toBeInTheDocument()
})

it("marks the shared dashboard controls with the compact visual density", () => {
  render(
    <>
      <Button>Save changes</Button>
      <Input aria-label="Project name" />
      <Label htmlFor="project-name">Project name label</Label>
      <Textarea aria-label="Description" />
      <Badge>Active</Badge>
      <Card>Card content</Card>
      <Table aria-label="Projects">
        <tbody>
          <tr><td>Project one</td></tr>
        </tbody>
      </Table>
    </>,
  )

  const saveChanges = screen.getByRole("button", { name: "Save changes" })
  expect(saveChanges).toHaveAttribute("data-density", "dashboard")
  expect(saveChanges).toHaveAttribute("data-typography", "regular")
  expect(screen.getByLabelText("Project name")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByText("Project name label")).toHaveAttribute("data-typography", "code")
  expect(screen.getByLabelText("Description")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByText("Active")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByText("Card content")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByRole("table", { name: "Projects" }).parentElement).toHaveAttribute("data-density", "dashboard")
})
