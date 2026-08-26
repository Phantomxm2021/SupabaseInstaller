import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Box, Rocket } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link } from 'react-router-dom'
import { apiFetch } from '../../api/client'
import { OperationPanel } from '../operations/OperationPanel'
import { AuthStep } from './AuthStep'
import { BasicStep } from './BasicStep'
import { DatabaseNetworkStep } from './DatabaseNetworkStep'
import { PresetStep } from './PresetStep'
import { ReviewStep } from './ReviewStep'
import { StorageFunctionsStep } from './StorageFunctionsStep'
import { defaultConfiguration, projectSchema, slugify, type ProjectForm } from './projectSchema'

const steps=['Basic','Preset & Services','Auth & SMTP','Storage & Functions','Database & Network','Review']
export function NewProjectPage() {
  const [step,setStep]=useState(0); const [operation,setOperation]=useState<{projectId:string;operationId:string}>();
  const form=useForm<ProjectForm>({resolver:zodResolver(projectSchema) as any,defaultValues:{name:'',slug:'',preset:'LIGHTWEIGHT',configuration:defaultConfiguration('LIGHTWEIGHT')}})
  const name=form.watch('name'); const values=form.watch()
  useEffect(()=>{form.setValue('slug',slugify(name),{shouldValidate:form.formState.isSubmitted})},[name,form])
  const create=useMutation({mutationFn:(v:ProjectForm)=>apiFetch<{projectId:string;operationId:string}>('/api/projects',{method:'POST',body:JSON.stringify({name:v.name,slug:v.slug,domain:v.configuration.general.domain,siteUrl:v.configuration.general.siteUrl,supabaseVersion:v.configuration.general.supabaseVersion,preset:v.preset,configuration:v.configuration,services:v.configuration.services})}),onSuccess:setOperation})
  if(operation)return <main className="page narrow-page"><div className="page-heading"><div><p className="eyebrow">Installation in progress</p><h1>Installing {values.name}</h1><p className="muted">You can leave this page. Progress is stored on the server.</p></div></div><OperationPanel operationId={operation.operationId} projectId={operation.projectId} projectName={values.name} onSucceeded={()=>undefined}/></main>
  const next=async()=>{const ok=step===0?await form.trigger(['name','slug','configuration.general.domain','configuration.general.siteUrl'] as any):step===4?await form.trigger():true;if(!ok)return;setStep(v=>Math.min(5,v+1))}
  return <main className="page narrow-page"><div className="page-heading"><div><p className="eyebrow">New runtime</p><h1>{step===5?'Review installation':'Create a project'}</h1><p className="muted">Configure the complete Supabase runtime before Docker resources are created.</p></div><span className="wizard-step">Step {step+1} of 6 · {steps[step]}</span></div><nav aria-label="Project setup steps" className="wizard-tabs">{steps.map((s,i)=><button type="button" key={s} aria-current={i===step?'step':undefined} disabled={i>step} onClick={()=>setStep(i)}>{i+1}. {s}</button>)}</nav><div className="wizard panel">{step===0&&<BasicStep form={form}/>} {step===1&&<PresetStep form={form}/>} {step===2&&<AuthStep form={form}/>} {step===3&&<StorageFunctionsStep form={form}/>} {step===4&&<DatabaseNetworkStep form={form}/>} {step===5&&<ReviewStep project={values}/>} {form.formState.errors.root&&<div className="alert error wizard-error">{String(form.formState.errors.root.message)}</div>}{create.error&&<div className="alert error wizard-error">{create.error.message}</div>}<div className="wizard-footer"><Link className="button secondary" to="/projects"><ArrowLeft size={15}/>Cancel</Link><div>{step>0&&<button className="button secondary" type="button" onClick={()=>setStep(v=>v-1)}><ArrowLeft size={15}/>Back</button>} {step===0&&<><button className="button secondary" type="button" onClick={()=>setStep(5)}>Review</button><button className="button primary" type="button" onClick={next}>Continue<ArrowRight size={15}/></button></>}{step>0&&step<5&&<button className="button primary" type="button" onClick={next}>Continue<ArrowRight size={15}/></button>}{step===5&&<button className="button primary" type="button" disabled={create.isPending} onClick={()=>create.mutate(form.getValues())}>{create.isPending?<Box size={15}/>:<Rocket size={15}/>} {create.isPending?'Creating operation…':'Install project'}</button>}</div></div></div></main>
}
