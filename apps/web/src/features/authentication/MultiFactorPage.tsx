import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver, type UseFormReturn } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AuthConfig, MFAConfig } from '../../api/types'
import { useResetOnServerRevision } from '../project/configuration/fields'
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
  return <main className="page auth-page auth-reference-page auth-mfa-reference-page dashboard-stack">
    <header className="page-heading"><div><h1>Multi-Factor Authentication (MFA)</h1><p className="muted">Require users to provide additional verification factors to authenticate.</p></div></header>
    <form className="auth-reference-form" onSubmit={form.handleSubmit((value) => context.requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
      <MFASection title="Multi-Factor Authentication (MFA)" save={<Button type="submit" disabled={!form.formState.isDirty}>Save changes</Button>}>
        <FactorRow id="totp-factor" label="TOTP (App Authenticator)" description="Control use of TOTP (App Authenticator) factors" enabled={mfa.totpEnrollEnabled && mfa.totpVerifyEnabled} onChange={(enabled) => { set('totpEnrollEnabled', enabled); set('totpVerifyEnabled', enabled) }} />
        <NumberRow id="max-factors" label="Maximum number of per-user MFA factors" description="How many MFA factors can be enrolled at once per user." form={form} field="mfa.maxEnrolledFactors" min={1} max={100} unit="factors" />
      </MFASection>
      <MFASection title="SMS MFA">
        <FactorRow id="phone-factor" label="Phone" description="Control use of phone factors" enabled={mfa.phoneEnrollEnabled && mfa.phoneVerifyEnabled} onChange={(enabled) => { set('phoneEnrollEnabled', enabled); set('phoneVerifyEnabled', enabled) }} />
        <NumberRow id="phone-otp-length" label="Phone OTP length" description="Number of digits in the SMS one-time password." form={form} field="mfa.phoneOtpLength" min={4} max={10} unit="digits" />
      </MFASection>
    </form>
  </main>
}

function MFASection({ title, children, save }: { title: string; children: React.ReactNode; save?: React.ReactNode }) { return <section className="auth-reference-section"><h2>{title}</h2><div className="auth-settings-card auth-mfa-card">{children}{save && <div className="auth-reference-card-footer">{save}</div>}</div></section> }
function FactorRow({ id, label, description, enabled, onChange }: { id: string; label: string; description: string; enabled: boolean; onChange: (value: boolean) => void }) { return <div className="auth-mfa-factor-row"><div><label htmlFor={id}>{label}</label><p>{description}</p></div><select id={id} aria-label={label} value={enabled ? 'enabled' : 'disabled'} onChange={(event) => onChange(event.target.value === 'enabled')}><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></div> }
function NumberRow({ id, label, description, form, field, min, max, unit }: { id: string; label: string; description: string; form: UseFormReturn<AuthConfig>; field: 'mfa.maxEnrolledFactors' | 'mfa.phoneOtpLength'; min: number; max: number; unit: string }) { return <div className="auth-mfa-number-row"><div><label htmlFor={id}>{label}</label><p>{description}</p></div><div className="relative"><Input id={id} aria-label={label} type="number" min={min} max={max} className="pr-20" {...form.register(field, { valueAsNumber: true })} /><span className="pointer-events-none absolute inset-y-0 right-4 flex items-center text-sm text-muted-foreground">{unit}</span></div></div> }
