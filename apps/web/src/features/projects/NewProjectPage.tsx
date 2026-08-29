import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Rocket } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useNavigate } from 'react-router-dom'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Spinner } from '@/components/ui/spinner'
import { PageHeader } from '@/components/app/PageHeader'
import { APIError, apiFetch } from '../../api/client'
import type { CreateProjectRequest, Project } from '../../api/types'
import { OperationPanel } from '../operations/OperationPanel'
import { BasicStep } from './BasicStep'
import { ReviewStep } from './ReviewStep'
import { defaultConfiguration, normalizeCreateConfiguration, projectSchema, slugify, type ProjectForm } from './projectSchema'
import { ServiceConfiguration } from './wizard/ServiceConfiguration'
import { InfrastructureReviewStep, type InfrastructureSectionId } from './wizard/InfrastructureReviewStep'
import { SecurityIntegrationsStep } from './wizard/SecurityIntegrationsStep'
import { useProjectIdentityAvailability } from './wizard/useProjectIdentityAvailability'
import { WizardStepFrame, type WizardStepDirection } from './wizard/WizardStepFrame'
import { wizardSteps } from './wizard/types'

export function NewProjectPage() {
  const [step, setStep] = useState(0); const [direction, setDirection] = useState<WizardStepDirection>('forward'); const [operation, setOperation] = useState<{ projectId: string; operationId: string }>(); const [openedInfrastructureSection, setOpenedInfrastructureSection] = useState<InfrastructureSectionId>(); const navigate = useNavigate()
  const queryClient = useQueryClient()
  const wizardHeader = useRef<HTMLDivElement>(null)
  const previousStep = useRef(step)
  const form = useForm<ProjectForm, any, ProjectForm>({ resolver: zodResolver(projectSchema) as any, mode: 'onChange', defaultValues: { name: '', slug: '', preset: 'LIGHTWEIGHT', configuration: defaultConfiguration('LIGHTWEIGHT') as any } })
  const name = form.watch('name'); const values = form.watch()
  useEffect(() => { form.setValue('slug', slugify(name), { shouldValidate: form.formState.isSubmitted, shouldDirty: !!name }) }, [name, form])
  useEffect(() => {
    const heading = wizardHeader.current?.querySelector<HTMLHeadingElement>('h1')
    if (heading) heading.tabIndex = -1
    if (previousStep.current !== step) heading?.focus()
    previousStep.current = step
  }, [step])
  const projects = useQuery({ queryKey: ['projects'], queryFn: () => apiFetch<{ projects: Project[] }>('/api/projects'), retry: false })
  const availability = useProjectIdentityAvailability(name, values.slug, projects.data?.projects, projects.error)
  const identityReady = availability.name.status === 'available' && availability.slug.status === 'available'
  const firstStepCanContinue = identityReady && form.formState.isValid
  const create = useMutation({ mutationFn: (value: ProjectForm) => { const configuration = normalizeCreateConfiguration(value.configuration); const body: CreateProjectRequest = { name: value.name, slug: value.slug, preset: value.preset, configuration }; return apiFetch<{ projectId: string; operationId: string }>('/api/projects', { method: 'POST', body: JSON.stringify(body) }) }, onSuccess: setOperation, onError: (error) => { if (error instanceof APIError && error.status === 409) { form.setError('root.server', { message: error.message }); queryClient.invalidateQueries({ queryKey: ['projects'] }); moveToStep(0) } } })
  if (operation) return <main className="page narrow-page"><div className="page-heading"><PageHeader eyebrow="Installation in progress" title={`Installing ${values.name}`} description="You can leave this page. Progress is stored on the server." /></div><OperationPanel operationId={operation.operationId} projectId={operation.projectId} projectName={values.name} onSucceeded={(projectId) => navigate(`/projects/${projectId}/overview`, { replace: true })} onDeleted={() => navigate('/projects', { replace: true })} /></main>
  const focusFirstInvalid = () => { window.setTimeout(() => document.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus(), 0) }
  const revealInfrastructureSection = (section: InfrastructureSectionId, focusTrigger = false) => { setOpenedInfrastructureSection(section); window.setTimeout(() => { if (focusTrigger) document.querySelector<HTMLElement>(`[data-infrastructure-section="${section}"] [data-slot="collapsible-trigger"]`)?.focus(); setOpenedInfrastructureSection(undefined) }, 0) }
  const moveToStep = (nextStep: number) => {
    setDirection(nextStep < step ? 'backward' : 'forward')
    setStep(nextStep)
  }
  const validateAnd = async (target: number) => {
    if (step === 0) {
      const valid = await form.trigger(['name', 'slug', 'configuration.general.siteUrl', 'configuration.general.studioUsername', 'configuration.general.studioPassword'])
      if (valid && identityReady) moveToStep(target)
      else focusFirstInvalid()
      return
    }
    if (await form.trigger()) moveToStep(target)
    else focusFirstInvalid()
  }
  const next = () => validateAnd(Math.min(wizardSteps.length - 1, step + 1))
  const install = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (await form.trigger()) await form.handleSubmit((value) => create.mutate(value), () => undefined)(event)
    else await form.handleSubmit(() => undefined, (errors) => { const invalidStep = stepForError(errors); if (invalidStep === 3) revealInfrastructureSection(infrastructureSectionForError(errors)); moveToStep(invalidStep); focusFirstInvalid() })(event)
  }
  const currentStep = wizardSteps[step]
  return <main className="page narrow-page"><div className="page-heading" ref={wizardHeader}><PageHeader eyebrow="New runtime" title={step === wizardSteps.length - 1 ? 'Review installation' : 'Create a project'} description="Configure the complete Supabase runtime before Docker resources are created." actions={<span className="wizard-step">Step {step + 1} of {wizardSteps.length} · {currentStep.label}</span>} /></div><Form {...form}><form className="wizard mt-3" onSubmit={install} noValidate><WizardStepFrame key={currentStep.id} step={currentStep.id} direction={direction}>{step === 0 && <BasicStep form={form} availability={availability} onRetryAvailability={() => { void projects.refetch() }} />}{step === 1 && <ServiceConfiguration form={form} />}{step === 2 && <SecurityIntegrationsStep form={form} />}{step === 3 && <div className="space-y-6"><InfrastructureReviewStep form={form} openedSection={openedInfrastructureSection} /><ReviewStep project={values} onEdit={(target) => moveToStep(wizardSteps.findIndex((item) => item.id === target))} onEditInfrastructure={() => revealInfrastructureSection('database-realtime', true)} /></div>}</WizardStepFrame>{(form.formState.errors.root || create.error) && <Alert variant="destructive" className="mt-4">{String(form.formState.errors.root?.message || create.error?.message)}</Alert>}<div className="wizard-footer"><Button variant="secondary" nativeButton={false} render={<Link to="/projects" />}><ArrowLeft size={15} />Cancel</Button><div className="flex gap-2">{step > 0 && <Button variant="secondary" type="button" onClick={() => moveToStep(step - 1)}><ArrowLeft size={15} />Back</Button>}{step < wizardSteps.length - 1 && <Button type="button" disabled={step === 0 && !firstStepCanContinue} onClick={next}>Continue<ArrowRight size={15} /></Button>}{step === wizardSteps.length - 1 && <Button type="submit" disabled={create.isPending} aria-busy={create.isPending}>{create.isPending ? <Spinner data-icon="inline-start" aria-label="Creating operation" /> : <Rocket size={15} />}{create.isPending ? 'Creating operation…' : 'Install project'}</Button>}</div></div></form></Form></main>
}

function stepForError(errors: Record<string, unknown>): number {
  const path = errorPath(errors)
  if (/^(name|slug|configuration\.general)/.test(path)) return 0
  if (path.startsWith('configuration.auth') || path.startsWith('configuration.storage') || path.startsWith('configuration.functions')) return 2
  if (path.startsWith('configuration.database') || path.startsWith('configuration.realtime') || path.startsWith('configuration.pooler') || path.startsWith('configuration.network')) return 3
  return 1
}

function infrastructureSectionForError(errors: Record<string, unknown>): InfrastructureSectionId {
  const path = errorPath(errors)
  if (path.startsWith('configuration.pooler')) return 'pooler'
  if (path.startsWith('configuration.network')) return 'gateway-network'
  return 'database-realtime'
}

function errorPath(errors: Record<string, unknown>): string {
  const find = (value: unknown, path: string[] = []): string[] | undefined => {
    if (!value || typeof value !== 'object') return undefined
    for (const [key, child] of Object.entries(value)) {
      if (key === 'message' || key === 'type' || key === 'ref') continue
      const next = [...path, key]
      if (child && typeof child === 'object' && 'message' in child && typeof (child as { message?: unknown }).message === 'string') return next
      const nested = find(child, next)
      if (nested) return nested
    }
    return undefined
  }
  return find(errors)?.join('.') ?? ''
}
