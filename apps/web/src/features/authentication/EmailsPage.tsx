import { zodResolver } from '@hookform/resolvers/zod'
import { ChevronRight, Mail, Server } from 'lucide-react'
import { useEffect, useState } from 'react'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { useForm, type Resolver } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { SMTPConfig } from '../../api/types'
import { NumberField, SecretEditor, TextField, Toggle, errorAt, useResetOnServerRevision } from '../project/configuration/fields'
import { smtpSchema } from '../project/configuration/schema'
import { useAuthenticationWorkspace, type AuthenticationWorkspaceContext } from './AuthenticationWorkspace'

const templates = [
  ['confirm-signup', 'Confirm sign up', 'Ask users to confirm their email address after signing up'],
  ['invite-user', 'Invite user', 'Invite someone to create an account'],
  ['magic-link', 'Magic link or OTP', 'Send a one-time sign-in link or one-time password'],
  ['change-email', 'Change email address', 'Ask users to verify their new email address after changing it'],
  ['reset-password', 'Reset password', 'Send a password reset link or code'],
  ['reauthentication', 'Reauthentication', 'Ask users to verify their identity before a sensitive operation'],
] as const

/** context is an intentionally small test seam; routed use always reads Outlet context. */
export function EmailsPage({ context: provided }: { context?: AuthenticationWorkspaceContext }) {
  const workspace = useAuthenticationWorkspace()
  const context = provided ?? workspace
  const [template, setTemplate] = useState<(typeof templates)[number]>()
  const [tab, setTab] = useState('templates')
  const [smtpDirty, setSMTPDirty] = useState(false)
  const [nextTab, setNextTab] = useState<string>()
  const changeTab = (next: string) => {
    if (next === tab) return
    if (tab === 'smtp' && smtpDirty) { setNextTab(next); return }
    setTab(next)
  }
  return <main className="page space-y-8">
    <header className="page-heading"><div><p className="eyebrow">Authentication</p><h1>Emails</h1><p className="muted">Configure what emails your users receive and how they are sent.</p></div></header>
    <Tabs value={tab} onValueChange={changeTab} className="gap-8">
      <TabsList aria-label="Email settings" variant="line" className="border-b border-border pb-1">
        <TabsTrigger value="templates">Templates</TabsTrigger>
        <TabsTrigger value="smtp">SMTP Settings</TabsTrigger>
      </TabsList>
      <TabsContent value="templates"><TemplateList onOpen={setTemplate} /></TabsContent>
      <TabsContent value="smtp"><SMTPSettings initial={context.auth.smtp} revision={context.revision} requestSave={context.requestSave} onDirty={setSMTPDirty} /></TabsContent>
    </Tabs>
    <TemplateSheet template={template} onClose={() => setTemplate(undefined)} />
    <AlertDialog open={Boolean(nextTab)} onOpenChange={(open) => !open && setNextTab(undefined)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Discard SMTP changes?</AlertDialogTitle><AlertDialogDescription>Your unsaved SMTP settings will be lost.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Keep editing</AlertDialogCancel><AlertDialogAction onClick={() => { setSMTPDirty(false); setTab(nextTab ?? 'templates'); setNextTab(undefined) }}>Discard changes</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </main>
}

function TemplateList({ onOpen }: { onOpen: (template: (typeof templates)[number]) => void }) {
  return <section className="space-y-4"><div><h2 className="text-xl font-semibold">Authentication</h2><p className="muted mt-1">Review the transactional messages sent by Supabase Auth.</p></div><div className="overflow-hidden rounded-xl border border-border bg-card">
    {templates.map((template) => <Button key={template[0]} type="button" variant="ghost" className="h-auto w-full justify-between rounded-none border-b border-border px-5 py-5 text-left last:border-b-0 hover:bg-muted/45" onClick={() => onOpen(template)} aria-label={template[1]}>
      <span><span className="block text-sm font-medium text-foreground">{template[1]}</span><span className="mt-1 block text-sm font-normal text-muted-foreground">{template[2]}</span></span><ChevronRight className="size-5 text-muted-foreground" aria-hidden="true" />
    </Button>)}
  </div></section>
}

function TemplateSheet({ template, onClose }: { template?: (typeof templates)[number]; onClose: () => void }) {
  return <Sheet open={Boolean(template)} onOpenChange={(open) => !open && onClose()}><SheetContent aria-describedby={undefined} className="w-full overflow-y-auto sm:max-w-xl"><SheetHeader><SheetTitle className="flex items-center gap-2"><Mail className="size-5" />{template?.[1]}</SheetTitle><SheetDescription>{template?.[2]}</SheetDescription></SheetHeader><div className="m-6 rounded-lg border border-border bg-muted/30 p-4 text-sm"><p className="font-medium">Template editing is not available</p><p className="mt-2 text-muted-foreground">This Manager version does not expose typed template fields or a safe runtime update endpoint. Email delivery settings below remain fully configurable; template changes are intentionally not presented as savable.</p></div><div className="flex justify-end px-6"><Button type="button" variant="outline" onClick={onClose}>Close</Button></div></SheetContent></Sheet>
}

function SMTPSettings({ initial, revision, requestSave, onDirty }: { initial: SMTPConfig; revision: number; requestSave: AuthenticationWorkspaceContext['requestSave']; onDirty: (dirty: boolean) => void }) {
  const form = useForm<SMTPConfig>({ resolver: zodResolver(smtpSchema) as Resolver<SMTPConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  useEffect(() => onDirty(form.formState.isDirty), [form.formState.isDirty, onDirty])
  const smtp = form.watch()
  return <form className="space-y-7" onSubmit={form.handleSubmit((value) => requestSave({ section: 'smtp', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
    <div><h2 className="text-xl font-semibold">SMTP settings</h2><p className="muted mt-1">Configure how authentication emails are delivered.</p></div>
    <section className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="border-b border-border p-5"><Toggle id="smtp-enabled" label="Enable custom SMTP" description="Send authentication emails through your own SMTP provider. Rate limits still apply." checked={smtp.enabled} onChange={(enabled) => form.setValue('enabled', enabled, { shouldDirty: true, shouldValidate: true })} /></div>
      <SettingsRow title="Sender details" description="Configure the sender information displayed in your users’ inboxes."><div className="grid gap-4"><TextField form={form} name="senderEmail" label="Sender email address" placeholder="no-reply@example.com" /><TextField form={form} name="senderName" label="Sender name" placeholder="Your project" /></div></SettingsRow>
      <SettingsRow title="SMTP provider settings" description="The SMTP password is write-only and is retained until you replace or remove it."><div className="grid gap-4"><TextField form={form} name="host" label="Host" placeholder="smtp.example.com" /><NumberField form={form} name="port" label="Port number" min={1} max={65535} /><TextField form={form} name="username" label="Username" /><SecretEditor label="Password" configured={smtp.passwordSet} secret={smtp.password} error={errorAt(form.formState.errors, 'password')} onChange={(password) => form.setValue('password', password, { shouldDirty: true, shouldValidate: true })} /></div></SettingsRow>
      <div className="flex items-center justify-between gap-4 p-5"><span className="flex items-center gap-2 text-sm text-muted-foreground"><Server className="size-4" />Changes are applied through the Auth-only operation flow.</span><Button type="submit" disabled={!form.formState.isDirty}>Save changes</Button></div>
    </section>
  </form>
}

function SettingsRow({ title, description, children }: { title: string; description: string; children: React.ReactNode }) { return <div className="grid gap-5 border-b border-border p-5 lg:grid-cols-[minmax(12rem,0.8fr)_minmax(0,1.2fr)]"><div><h3 className="font-medium">{title}</h3><p className="mt-1 text-sm text-muted-foreground">{description}</p></div>{children}</div> }
