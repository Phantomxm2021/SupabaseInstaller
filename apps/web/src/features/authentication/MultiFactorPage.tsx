import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver, type UseFormReturn } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AuthConfig, MFAConfig } from '../../api/types'
import { Toggle, useResetOnServerRevision } from '../project/configuration/fields'
import { authSchema } from '../project/configuration/schema'
import { useAuthenticationWorkspace, type AuthenticationWorkspaceContext } from './AuthenticationWorkspace'

const defaults: MFAConfig = { totpEnrollEnabled: true, totpVerifyEnabled: true, phoneEnrollEnabled: false, phoneVerifyEnabled: false, maxEnrolledFactors: 10, phoneOtpLength: 6 }

export function MultiFactorPage({ context: provided }: { context?: AuthenticationWorkspaceContext }) {
  const workspace = useAuthenticationWorkspace()
  const context = provided ?? workspace
  const initial: AuthConfig = { ...context.auth, mfa: { ...defaults, ...context.auth.mfa } }
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, context.revision)
  const mfa = form.watch('mfa')
  const set = <K extends keyof MFAConfig>(field: K, value: MFAConfig[K]) => form.setValue(`mfa.${field}` as never, value as never, { shouldDirty: true, shouldValidate: true })
  return <main className="page space-y-12">
    <header className="page-heading"><div><p className="eyebrow">Authentication</p><h1>Multi-Factor Authentication (MFA)</h1><p className="muted">Require users to provide additional verification factors to authenticate.</p></div></header>
    <form className="space-y-12" onSubmit={form.handleSubmit((value) => context.requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
      <MFASection title="Multi-Factor Authentication (MFA)">
        <Toggle id="totp-enrollment" label="TOTP (App Authenticator) enrollment" description="Allow users to enroll an authenticator application as an additional factor." checked={mfa.totpEnrollEnabled} onChange={(value) => set('totpEnrollEnabled', value)} />
        <Toggle id="totp-verification" label="TOTP (App Authenticator) verification" description="Allow enrolled authenticator factors to verify sign-ins." checked={mfa.totpVerifyEnabled} onChange={(value) => set('totpVerifyEnabled', value)} />
        <NumberRow id="max-factors" label="Maximum number of per-user MFA factors" description="How many MFA factors can be enrolled at once per user." form={form} field="mfa.maxEnrolledFactors" min={1} max={100} unit="factors" />
      </MFASection>
      <MFASection title="SMS MFA">
        <Toggle id="phone-mfa-enrollment" label="Phone MFA enrollment" description="Allow users to enroll a phone number as a multi-factor authentication method." checked={mfa.phoneEnrollEnabled} onChange={(value) => set('phoneEnrollEnabled', value)} />
        <Toggle id="phone-mfa-verification" label="Phone MFA verification" description="Allow enrolled phone factors to verify sign-ins with an SMS code." checked={mfa.phoneVerifyEnabled} onChange={(value) => set('phoneVerifyEnabled', value)} />
        <NumberRow id="phone-otp-length" label="Phone OTP length" description="Number of digits in the SMS one-time password." form={form} field="mfa.phoneOtpLength" min={4} max={10} unit="digits" />
      </MFASection>
      <div className="flex justify-end"><Button type="submit" disabled={!form.formState.isDirty}>Save changes</Button></div>
    </form>
  </main>
}

function MFASection({ title, children }: { title: string; children: React.ReactNode }) { return <section className="space-y-4"><h2 className="text-xl font-semibold">{title}</h2><div className="overflow-hidden rounded-xl border border-border bg-card"><div className="grid gap-0 sm:grid-cols-2 [&>div]:rounded-none [&>div]:border-0 [&>div]:border-b [&>div]:border-border [&>div]:p-5">{children}</div></div></section> }
function NumberRow({ id, label, description, form, field, min, max, unit }: { id: string; label: string; description: string; form: UseFormReturn<AuthConfig>; field: 'mfa.maxEnrolledFactors' | 'mfa.phoneOtpLength'; min: number; max: number; unit: string }) { return <div className="sm:col-span-2 grid gap-4 sm:grid-cols-[minmax(16rem,1.2fr)_minmax(14rem,0.8fr)]"><div><label htmlFor={id} className="text-sm font-medium">{label}</label><p className="mt-1 text-sm text-muted-foreground">{description}</p></div><div className="relative"><Input id={id} aria-label={label} type="number" min={min} max={max} className="pr-20" {...form.register(field, { valueAsNumber: true })} /><span className="pointer-events-none absolute inset-y-0 right-4 flex items-center text-sm text-muted-foreground">{unit}</span></div></div> }
