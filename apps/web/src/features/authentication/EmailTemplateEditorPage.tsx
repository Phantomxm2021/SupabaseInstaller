import { zodResolver } from '@hookform/resolvers/zod'
import { ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { Link, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Toggle, useResetOnServerRevision } from '../project/configuration/fields'
import { mailerTemplateSchema } from '../project/configuration/schema'
import { defaultMailerConfiguration } from '../projects/projectSchema'
import { useAuthenticationWorkspace, type AuthenticationWorkspaceContext } from './AuthenticationWorkspace'
import { emailNotifications, emailTemplates } from './EmailsPage'
import type { EmailTemplateConfig, MailerConfig } from '../../api/types'

type TemplateKey = (typeof emailTemplates)[number][0] | (typeof emailNotifications)[number][0]
type Info = { title: string; description: string; notification?: keyof MailerConfig['notifications']; template?: keyof MailerConfig['templates'] }
function templateInfo(key: string): Info | undefined { const t = emailTemplates.find(([slug]) => slug === key); if (t) return { title: t[2], description: t[3], template: t[1] }; const n = emailNotifications.find(([slug]) => slug === key); return n ? { title: n[2], description: n[3], notification: n[1] } : undefined }

export function EmailTemplateEditorPage({ context: provided, templateKey: keyProp }: { context?: AuthenticationWorkspaceContext; templateKey?: TemplateKey }) {
  const workspace = useAuthenticationWorkspace(); const context = provided ?? workspace; const { templateKey: routeKey } = useParams(); const info = templateInfo(keyProp ?? routeKey ?? '')
  if (!info) return <main className="page"><h1>Template not found</h1><Link to={`/projects/${context.projectId}/authentication/emails`}>Back to Emails</Link></main>
  const defaults = defaultMailerConfiguration(); const initial = info.template ? context.auth.mailer.templates[info.template] : context.auth.mailer.notifications[info.notification!].template; const fallback = info.template ? defaults.templates[info.template] : defaults.notifications[info.notification!].template
  return <TemplateEditor context={context} info={info} initial={initial} fallback={fallback} />
}

function TemplateEditor({ context, info, initial, fallback }: { context: AuthenticationWorkspaceContext; info: Info; initial: EmailTemplateConfig; fallback: EmailTemplateConfig }) {
  const enabled = info.notification ? context.auth.mailer.notifications[info.notification].enabled : undefined
  const form = useForm<EmailTemplateConfig & { enabled?: boolean }>({ resolver: zodResolver(mailerTemplateSchema) as Resolver<EmailTemplateConfig & { enabled?: boolean }>, defaultValues: { ...initial, ...(info.notification ? { enabled } : {}) } })
  useResetOnServerRevision(form as never, { ...initial, ...(info.notification ? { enabled } : {}) }, context.revision)
  const [preview, setPreview] = useState(false); const templateUrl = form.watch('templateUrl')
  const submit = (value: EmailTemplateConfig & { enabled?: boolean }) => { const mailer = structuredClone(context.auth.mailer); const template = { subject: value.subject, templateUrl: value.templateUrl }; if (info.template) mailer.templates[info.template] = template; else { mailer.notifications[info.notification!].template = template; mailer.notifications[info.notification!].enabled = Boolean(value.enabled) }; context.requestSave({ section: 'auth', value: { ...context.auth, mailer }, dirty: { mailer: info.template ? { templates: { [info.template]: form.formState.dirtyFields } } : { notifications: { [info.notification!]: form.formState.dirtyFields } } }, setError: (name, message) => form.setError(name.replace(/^mailer\.(templates|notifications)\.[^.]+\./, '') as never, { type: 'server', message }) }) }
  return <main className="page space-y-10"><nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm text-muted-foreground"><Link to={`/projects/${context.projectId}/authentication/emails`}>Emails</Link><ChevronRight className="size-4" /><span className="text-foreground">{info.title}</span></nav><header className="page-heading"><div><h1>{info.title}</h1><p className="muted">{info.description}</p></div></header><section className="space-y-4"><h2 className="text-xl font-semibold">{info.notification ? 'Configuration' : 'Template'}</h2><form className="overflow-hidden rounded-xl border border-border bg-card" onSubmit={form.handleSubmit(submit)}>{info.notification && <div className="border-b border-border p-5"><Toggle label="Enable notification" description="Send this email to users when triggered." checked={Boolean(form.watch('enabled'))} onChange={(value) => form.setValue('enabled', value, { shouldDirty: true })} /></div>}<div className="border-b border-border p-5"><label className="grid gap-2 text-sm font-medium">Subject<Input aria-label="Subject" {...form.register('subject')} /></label>{form.formState.errors.subject && <p className="mt-2 text-sm text-destructive">{form.formState.errors.subject.message}</p>}</div><div className="p-5"><div className="mb-3 flex items-center justify-between"><label htmlFor="template-url" className="text-sm font-medium">Template URL</label><Button type="button" size="sm" variant="outline" disabled={!templateUrl} onClick={() => setPreview((value) => !value)}>{preview ? 'Edit URL' : 'Preview'}</Button></div>{preview && templateUrl ? <iframe title="Email template preview" sandbox="" className="mb-4 h-80 w-full rounded-lg border border-border bg-white" src={templateUrl} /> : null}<Input id="template-url" aria-label="Template URL" placeholder="https://templates.example.com/confirmation.html" {...form.register('templateUrl')} /><p className="mt-2 text-sm text-muted-foreground">Optional HTTP(S) URL fetched by GoTrue. Leave empty to use its built-in template.</p>{form.formState.errors.templateUrl && <p className="mt-2 text-sm text-destructive">{form.formState.errors.templateUrl.message}</p>}</div><div className="flex justify-between border-t border-border p-5"><Button type="button" variant="outline" onClick={() => form.reset({ ...fallback, ...(info.notification ? { enabled: false } : {}) })}>Reset template</Button><Button type="submit" disabled={!form.formState.isDirty}>Save changes</Button></div></form></section></main>
}
