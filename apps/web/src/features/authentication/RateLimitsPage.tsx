import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import { BookOpen } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { AuthConfig, RateLimitConfig } from '../../api/types'
import { authSchema } from '../project/configuration/schema'
import { useResetOnServerRevision } from '../project/configuration/fields'
import { useAuthenticationWorkspace, type AuthenticationWorkspaceContext } from './AuthenticationWorkspace'

const defaults: RateLimitConfig = { emailSent: 30, smsSent: 30, tokenRefresh: 150, tokenVerification: 30, anonymousUsers: 30, signupsAndSignins: 30 }

const rows: Array<{ field: keyof RateLimitConfig; title: string; description: string; unit: string; note?: string }> = [
  { field: 'emailSent', title: 'Rate limit for sending emails', description: 'Number of emails that can be sent per hour from your project', unit: 'emails/h' },
  { field: 'smsSent', title: 'Rate limit for sending SMS messages', description: 'Number of SMS messages that can be sent per hour from your project', unit: 'sms/h' },
  { field: 'tokenRefresh', title: 'Rate limit for token refreshes', description: 'Number of sessions that can be refreshed in a 5 minute interval per IP address', unit: 'requests/5 min', note: '12× this value per hour' },
  { field: 'tokenVerification', title: 'Rate limit for token verifications', description: 'Number of OTP and magic link verifications that can be made in a 5 minute interval per IP address', unit: 'requests/5 min', note: '12× this value per hour' },
  { field: 'anonymousUsers', title: 'Rate limit for anonymous users', description: 'Number of anonymous sign-ins that can be made per hour per IP address', unit: 'requests/h' },
  { field: 'signupsAndSignins', title: 'Rate limit for sign-ups and sign-ins', description: 'Number of sign-up and sign-in requests that can be made in a 5 minute interval per IP address (excludes anonymous users)', unit: 'requests/5 min', note: '12× this value per hour' },
]

/** The optional context keeps the complete settings page easy to exercise without routing. */
export function RateLimitsPage({ context: provided }: { context?: AuthenticationWorkspaceContext }) {
  const workspace = useAuthenticationWorkspace()
  const context = provided ?? workspace
  const initial: AuthConfig = { ...context.auth, rateLimits: { ...defaults, ...context.auth.rateLimits } }
  const form = useForm<AuthConfig>({ resolver: zodResolver(authSchema) as Resolver<AuthConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, context.revision)
  const limits = form.watch('rateLimits')
  return <main className="page auth-page auth-reference-page">
    <header className="page-heading auth-reference-heading"><div><h1>Rate Limits</h1><p className="muted">Safeguard against bursts of incoming traffic to prevent abuse and maximize stability.</p></div><a className="auth-docs-link" href="https://supabase.com/docs/guides/platform/going-into-prod#rate-limiting-resource-allocation--abuse-prevention" target="_blank" rel="noreferrer"><BookOpen aria-hidden="true" />Docs</a></header>
    <form onSubmit={form.handleSubmit((value) => context.requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
      <section className="auth-settings-card auth-rate-limit-card auth-rate-limits-card" aria-label="Authentication rate limits">
        {rows.map((row, index) => <div key={row.field} className={index ? 'auth-rate-limit-row border-t border-border' : 'auth-rate-limit-row'}>
          <div><h2>{row.title}</h2><p className="text-muted-foreground">{row.description}</p></div>
          <div><div className="relative"><label className="sr-only" htmlFor={`rate-${row.field}`}>{row.title}</label><Input id={`rate-${row.field}`} type="number" min={1} max={1_000_000} className="pr-28" {...form.register(`rateLimits.${row.field}`, { valueAsNumber: true })} /><span className="pointer-events-none absolute inset-y-0 right-4 flex items-center text-sm text-muted-foreground">{row.unit}</span></div>{row.note && <p className="auth-rate-limit-note">{row.note.replace('12× this value', String((limits[row.field] || 0) * 12))}</p>}</div>
        </div>)}
        <div className="auth-reference-card-footer"><Button type="submit" disabled={!form.formState.isDirty}>Save changes</Button></div>
      </section>
    </form>
  </main>
}
