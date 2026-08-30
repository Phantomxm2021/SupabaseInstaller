import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"

import { SettingRow } from "./SettingRow"

function ToggleableSettingRow() {
  const [enabled, setEnabled] = useState(true)

  return (
    <SettingRow
      label="Enable realtime"
      description="Accept live database events."
      checked={enabled}
      onCheckedChange={setEnabled}
    >
      <p>Realtime configuration</p>
    </SettingRow>
  )
}

it("keeps its usable collapsible trigger open when the named switch changes", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent)
  const user = userEvent.setup()

  render(<ToggleableSettingRow />)

  const trigger = screen.getByRole("button", { name: /enable realtime/i })
  const toggle = screen.getByRole("switch", { name: "Enable realtime", checked: true })
  const header = trigger.parentElement

  expect(header?.parentElement?.parentElement).toHaveAttribute("data-density", "dashboard")
  expect(header).toHaveClass(
    "grid",
    "grid-cols-[minmax(0,1fr)_auto]",
    "!grid-rows-1",
    "items-center",
  )
  expect(trigger.nextElementSibling).toBe(toggle)
  expect(trigger.querySelector('[data-slot="setting-row-label"]')).toHaveProperty("tagName", "SPAN")
  expect(trigger.querySelector('[data-slot="setting-row-description"]')).toHaveProperty("tagName", "SPAN")

  await user.click(trigger)
  expect(screen.getByText("Realtime configuration")).toBeVisible()

  await user.click(toggle)
  expect(screen.getByRole("switch", { name: "Enable realtime", checked: false })).toBeInTheDocument()
  expect(trigger).toHaveAttribute("aria-expanded", "true")
})
