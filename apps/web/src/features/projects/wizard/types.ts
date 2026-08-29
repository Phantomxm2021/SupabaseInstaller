export const wizardSteps = [
  { id: 'identity', label: 'Server details' },
  { id: 'services', label: 'Services' },
  { id: 'integrations', label: 'Security & integrations' },
  { id: 'review', label: 'Review & install' },
] as const

export type WizardStepId = (typeof wizardSteps)[number]['id']

export type Availability = {
  status: 'idle' | 'checking' | 'available' | 'conflict' | 'unavailable'
  message?: string
}
