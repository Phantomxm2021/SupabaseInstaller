import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Box, Rocket } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link } from 'react-router-dom'
import { apiFetch } from '../../api/client'
import { OperationPanel } from '../operations/OperationPanel'
import { BasicStep } from './BasicStep'
import { PresetStep } from './PresetStep'
import { ReviewStep } from './ReviewStep'
import { lightweightServices, projectSchema, slugify, type ProjectForm } from './projectSchema'

export function NewProjectPage() {
  const [review, setReview] = useState(false)
  const [operationId, setOperationId] = useState('')
  const form = useForm<ProjectForm>({ resolver: zodResolver(projectSchema), defaultValues: { name: '', slug: '', domain: '', siteUrl: '' } })
  const name = form.watch('name')
  useEffect(() => { form.setValue('slug', slugify(name), { shouldValidate: form.formState.isSubmitted }) }, [name, form])
  const create = useMutation({
    mutationFn: (values: ProjectForm) => apiFetch<{ projectId: string; operationId: string }>('/api/projects', { method: 'POST', body: JSON.stringify({ ...values, supabaseVersion: 'self-hosted/v0.8.0', preset: 'LIGHTWEIGHT', services: lightweightServices }) }),
    onSuccess: (result) => setOperationId(result.operationId),
  })
  const values = form.getValues()

  if (operationId) {
    return <main className="page narrow-page"><div className="page-heading"><div><p className="eyebrow">Installation in progress</p><h1>Installing {values.name}</h1><p className="muted">You can leave this page. Progress is stored on the server.</p></div></div><OperationPanel operationId={operationId} projectName={values.name} /></main>
  }

  return (
    <main className="page narrow-page">
      <div className="page-heading"><div><p className="eyebrow">New runtime</p><h1>{review ? 'Review installation' : 'Create a project'}</h1><p className="muted">{review ? 'Confirm the isolated runtime before Docker resources are created.' : 'A production-oriented Lightweight project takes three fields.'}</p></div><span className="wizard-step">{review ? 'Review' : 'Configure'}</span></div>
      {!review ? (
        <form className="wizard panel" onSubmit={form.handleSubmit(() => setReview(true))}>
          <BasicStep form={form} />
          <PresetStep />
          <div className="wizard-footer"><Link className="button secondary" to="/projects"><ArrowLeft size={15} />Cancel</Link><button className="button primary" type="submit">Review<ArrowRight size={15} /></button></div>
        </form>
      ) : (
        <div className="wizard panel">
          <ReviewStep project={values} />
          {create.error && <div className="alert error wizard-error">{create.error.message}</div>}
          <div className="wizard-footer"><button className="button secondary" onClick={() => setReview(false)}><ArrowLeft size={15} />Back</button><button className="button primary" disabled={create.isPending} onClick={() => create.mutate(values)}>{create.isPending ? <Box size={15} /> : <Rocket size={15} />}{create.isPending ? 'Creating operation…' : 'Install project'}</button></div>
        </div>
      )}
    </main>
  )
}
