import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
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
  return <main className="page space-y-8">
    <header className="page-heading"><div><p className="eyebrow">Authentication</p><h1>Rate Limits</h1><p className="muted">Safeguard against bursts of incoming traffic to prevent abuse and maximize stability.</p></div></header>
    <form onSubmit={form.handleSubmit((value) => context.requestSave({ section: 'auth', value, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))}>
      <section className="overflow-hidden rounded-xl border border-border bg-card" aria-label="Authentication rate limits">
        {rows.map((row, index) => <div key={row.field} className={index ? 'grid gap-5 border-t border-border p-5 lg:grid-cols-[minmax(18rem,1.2fr)_minmax(20rem,0.8fr)]' : 'grid gap-5 p-5 lg:grid-cols-[minmax(18rem,1.2fr)_minmax(20rem,0.8fr)]'}>
          <div><h2 className="font-medium">{row.title}</h2><p className="mt-1 text-sm text-muted-foreground">{row.description}</p></div>
          <div><div className="relative"><label className="sr-only" htmlFor={`rate-${row.field}`}>{row.title}</label><Input id={`rate-${row.field}`} type="number" min={1} max={1_000_000} className="pr-28" {...form.register(`rateLimits.${row.field}`, { valueAsNumber: true })} /><span className="pointer-events-none absolute inset-y-0 right-4 flex items-center text-sm text-muted-foreground">{row.unit}</span></div>{row.note && <p className="mt-2 text-right text-sm text-muted-foreground">{row.note.replace('12× this value', String((limits[row.field] || 0) * 12))}</p>}</div>
        </div>)}
        <div className="flex justify-end border-t border-border p-5"><Button type="submit" disabled={!form.formState.isDirty}>Save changes</Button></div>
      </section>
    </form>
  </main>
}
