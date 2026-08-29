import { render, screen } from "@testing-library/react"

import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "./command"
import { Spinner } from "./spinner"

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
