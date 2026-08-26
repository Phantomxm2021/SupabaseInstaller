import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import type { SMTPConfig } from '../../../api/types'
import { smtpSchema } from './schema'
import { SectionCard, Toggle, TextField, NumberField, SecretEditor, SectionSaveButton, useResetOnServerRevision } from './fields'

export function SMTPSection({ initial, revision, onSave }: { initial: SMTPConfig; revision: number; onSave: (value: SMTPConfig, dirty: unknown, setError?: (name: string, message: string) => void) => void }) {
  const form = useForm<SMTPConfig>({ resolver: zodResolver(smtpSchema) as Resolver<SMTPConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const smtp = form.watch()
  const setError = (name: string, message: string) => form.setError(name as never, { type: 'server', message })
  const secretError = String((form.formState.errors.password as any)?.message ?? '') || undefined
  return <form id="configuration-smtp-form" onSubmit={form.handleSubmit((value) => onSave(value, form.formState.dirtyFields, setError))} className="space-y-5">
    <SectionCard title="Email & SMTP" description="Custom SMTP is write-only. A configured password is retained until you explicitly replace or remove it.">
      <Toggle id="smtp-enabled" label="Enable custom SMTP" checked={smtp.enabled} onChange={(value) => form.setValue('enabled', value, { shouldDirty: true, shouldValidate: true })} />
      {(smtp.enabled || smtp.passwordSet) && <div className="grid gap-4 md:grid-cols-2">
        {smtp.enabled && <><TextField form={form} name="host" label="Host" /><NumberField form={form} name="port" label="Port" min={1} max={65535} /><TextField form={form} name="username" label="Username" /></>}
        <SecretEditor label="Password" configured={smtp.passwordSet} secret={smtp.password} error={secretError} onChange={(value) => form.setValue('password', value, { shouldDirty: true, shouldValidate: true })} />
        {smtp.enabled && <><TextField form={form} name="senderEmail" label="Sender email" /><TextField form={form} name="senderName" label="Sender name" /></>}
      </div>}
    </SectionCard>
    <SectionSaveButton label="Email & SMTP" disabled={!form.formState.isDirty} />
  </form>
}
