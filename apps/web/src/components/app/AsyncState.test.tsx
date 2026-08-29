import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { AsyncState } from "./AsyncState"

it("renders loading, error retry, and empty action variants", async () => {
  const user = userEvent.setup()
  const onRetry = vi.fn()

  const { rerender } = render(<AsyncState variant="loading" />)
  expect(screen.getByLabelText("Loading")).toHaveAttribute("data-slot", "skeleton")

  rerender(
    <AsyncState
      variant="error"
      title="Could not load projects"
      onRetry={onRetry}
    />,
  )
  expect(screen.getByRole("alert")).toHaveTextContent("Could not load projects")
  await user.click(screen.getByRole("button", { name: "Retry" }))
  expect(onRetry).toHaveBeenCalledOnce()

  rerender(
    <AsyncState
      variant="empty"
      title="No projects yet"
      description="Create one to get started."
      action={<button type="button">New project</button>}
    />,
  )
  expect(screen.getByRole("heading", { name: "No projects yet" })).toBeInTheDocument()
  expect(screen.getByRole("button", { name: "New project" })).toBeInTheDocument()
})
