import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Box, Rocket } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useNavigate } from 'react-router-dom'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { apiFetch } from '../../api/client'
import type { CreateProjectRequest } from '../../api/types'
import { OperationPanel } from '../operations/OperationPanel'
import { AuthStep } from './AuthStep'
import { BasicStep } from './BasicStep'
import { DatabaseNetworkStep } from './DatabaseNetworkStep'
import { PresetStep } from './PresetStep'
import { ReviewStep } from './ReviewStep'
import { StorageFunctionsStep } from './StorageFunctionsStep'
import { defaultConfiguration, normalizeCreateConfiguration, projectSchema, slugify, type ProjectForm } from './projectSchema'

const steps = ['Basic', 'Preset & Services', 'Auth & SMTP', 'Storage & Functions', 'Database & Network', 'Review']
export function NewProjectPage() {
  const [step, setStep] = useState(0); const [operation, setOperation] = useState<{ projectId: string; operationId: string }>(); const navigate = useNavigate()
  const form = useForm<ProjectForm, any, ProjectForm>({ resolver: zodResolver(projectSchema) as any, defaultValues: { name: '', slug: '', preset: 'LIGHTWEIGHT', configuration: defaultConfiguration('LIGHTWEIGHT') as any } })
  const name = form.watch('name'); const values = form.watch()
  useEffect(() => { form.setValue('slug', slugify(name), { shouldValidate: form.formState.isSubmitted, shouldDirty: !!name }) }, [name, form])
  const create = useMutation({ mutationFn: (value: ProjectForm) => { const configuration = normalizeCreateConfiguration(value.configuration); const body: CreateProjectRequest = { name: value.name, slug: value.slug, domain: configuration.general.domain, siteUrl: configuration.general.siteUrl, supabaseVersion: configuration.general.supabaseVersion, preset: value.preset, configuration, services: configuration.services }; return apiFetch<{ projectId: string; operationId: string }>('/api/projects', { method: 'POST', body: JSON.stringify(body) }) }, onSuccess: setOperation })
  if (operation) return <main className="page narrow-page"><div className="page-heading"><div><p className="eyebrow">Installation in progress</p><h1>Installing {values.name}</h1><p className="muted">You can leave this page. Progress is stored on the server.</p></div></div><OperationPanel operationId={operation.operationId} projectId={operation.projectId} projectName={values.name} onSucceeded={(projectId) => navigate(`/projects/${projectId}/overview`, { replace: true })} /></main>
  const validateAnd = async (target: number) => {
    const scope = target === 5 ? undefined : step === 0 ? ['name', 'slug', 'configuration.general.domain', 'configuration.general.siteUrl'] : undefined
    await form.trigger(scope as any)
    await form.handleSubmit(() => setStep(target), () => undefined)()
  }
  const next = () => validateAnd(Math.min(5, step + 1))
  const install = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); await form.trigger(); await form.handleSubmit((value) => create.mutate(value), (errors) => setStep(stepForError(errors)))(event) }
  const tab = (target: number) => { if (target <= step) setStep(target); else validateAnd(target) }
  return <main className="page narrow-page"><div className="page-heading"><div><p className="eyebrow">New runtime</p><h1>{step === 5 ? 'Review installation' : 'Create a project'}</h1><p className="muted">Configure the complete Supabase runtime before Docker resources are created.</p></div><span className="wizard-step">Step {step + 1} of 6 · {steps[step]}</span></div><Form {...form}><form className="wizard mt-3" onSubmit={install} noValidate><Tabs value={`step-${step}`} onValueChange={(value) => tab(Number(value.replace('step-', '')))}><TabsList className="grid h-auto w-full grid-cols-2 gap-1 md:grid-cols-6">{steps.map((label, index) => <TabsTrigger key={label} value={`step-${index}`} disabled={index > step + 1}>{index + 1}. {label}</TabsTrigger>)}</TabsList></Tabs>{step === 0 && <BasicStep form={form} />}{step === 1 && <PresetStep form={form} />}{step === 2 && <AuthStep form={form} />}{step === 3 && <StorageFunctionsStep form={form} />}{step === 4 && <DatabaseNetworkStep form={form} />}{step === 5 && <ReviewStep project={values} />}{(form.formState.errors.root || create.error) && <Alert variant="destructive" className="mt-4">{String(form.formState.errors.root?.message || create.error?.message)}</Alert>}<div className="wizard-footer"><Button variant="secondary" nativeButton={false} render={<Link to="/projects" />}><ArrowLeft size={15} />Cancel</Button><div className="flex gap-2">{step > 0 && <Button variant="secondary" type="button" onClick={() => setStep((value) => value - 1)}><ArrowLeft size={15} />Back</Button>}{step < 5 && <><Button variant="secondary" type="button" onClick={() => validateAnd(5)}>Review</Button><Button type="button" onClick={next}>Continue<ArrowRight size={15} /></Button></>}{step === 5 && <Button type="submit" disabled={create.isPending}>{create.isPending ? <Box size={15} /> : <Rocket size={15} />}{create.isPending ? 'Creating operation…' : 'Install project'}</Button>}</div></div></form></Form></main>
}

function stepForError(errors: Record<string, unknown>): number {
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
  const path = find(errors)?.join('.') ?? ''
  if (/^(name|slug|configuration\.general)/.test(path)) return 0
  if (path.startsWith('configuration.auth')) return 2
  if (path.startsWith('configuration.storage') || path.startsWith('configuration.functions')) return 3
  if (path.startsWith('configuration.database') || path.startsWith('configuration.pooler') || path.startsWith('configuration.network')) return 4
  return 1
}
