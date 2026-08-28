import type { ReactNode } from 'react'
import type { WizardStepId } from './types'

export type WizardStepDirection = 'forward' | 'backward'

export function WizardStepFrame({
  step,
  direction,
  children,
}: {
  step: WizardStepId
  direction: WizardStepDirection
  children: ReactNode
}) {
  return (
    <section
      key={step}
      className="wizard-step-frame"
      data-direction={direction}
      data-testid="wizard-step-frame"
    >
      {children}
    </section>
  )
}
