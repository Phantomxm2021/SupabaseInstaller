import { render, screen } from "@testing-library/react"

import { Button } from "@/components/ui/button"

import { PageHeader } from "./PageHeader"

it("renders a semantic page title with its action slot", () => {
  render(
    <PageHeader
      eyebrow="Projects"
      title="Create a project"
      description="Choose the infrastructure for your new project."
      actions={<Button>Save draft</Button>}
    />,
  )

  expect(screen.getByRole("banner")).toBeInTheDocument()
  expect(
    screen.getByRole("heading", { level: 1, name: "Create a project" }),
  ).toBeInTheDocument()
  expect(screen.getByText("Projects")).toBeInTheDocument()
  expect(
    screen.getByText("Choose the infrastructure for your new project."),
  ).toBeInTheDocument()
  expect(screen.getByRole("button", { name: "Save draft" })).toBeInTheDocument()
})
