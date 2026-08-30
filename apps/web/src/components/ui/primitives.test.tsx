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

  expect(screen.getByRole("button", { name: "Save changes" })).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByLabelText("Project name")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByLabelText("Description")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByText("Active")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByText("Card content")).toHaveAttribute("data-density", "dashboard")
  expect(screen.getByRole("table", { name: "Projects" }).parentElement).toHaveAttribute("data-density", "dashboard")
})
