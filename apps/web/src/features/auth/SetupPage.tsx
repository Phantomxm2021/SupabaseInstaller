import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { Check, Copy, Database, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { apiFetch } from '../../api/client'

const schema = z.object({
  username: z.string().min(3).max(64).regex(/^[A-Za-z0-9][A-Za-z0-9._-]+$/),
  password: z.string().min(12, 'Password must contain at least 12 characters'),
  confirmPassword: z.string(),
}).refine((value) => value.password === value.confirmPassword, { message: 'Passwords do not match', path: ['confirmPassword'] })

type SetupForm = z.infer<typeof schema>

export function SetupPage() {
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [copied, setCopied] = useState(false)
  const form = useForm<SetupForm>({ resolver: zodResolver(schema), defaultValues: { username: '', password: '', confirmPassword: '' } })
  const setup = useMutation({
    mutationFn: (values: SetupForm) => apiFetch<{ recoveryCodes: string[] }>('/api/setup', { method: 'POST', body: JSON.stringify({ username: values.username, password: values.password }) }),
    onSuccess: (result) => setRecoveryCodes(result.recoveryCodes),
  })

  if (recoveryCodes.length > 0) {
    return (
      <main className="auth-layout">
        <section className="auth-card recovery-card">
          <div className="brand-mark"><ShieldCheck size={22} /></div>
          <p className="eyebrow">Account recovery</p>
          <h1>Save your recovery codes</h1>
          <p className="muted">These codes are shown once. Store them somewhere private before continuing.</p>
          <ul className="recovery-grid">
            {recoveryCodes.map((code) => <li key={code}><code>{code}</code></li>)}
          </ul>
          <button className="button secondary full" type="button" onClick={async () => { await navigator.clipboard?.writeText(recoveryCodes.join('\n')); setCopied(true) }}>
            {copied ? <Check size={16} /> : <Copy size={16} />} {copied ? 'Copied' : 'Copy all codes'}
          </button>
          <a className="button primary full" href="/login">Continue to sign in</a>
        </section>
      </main>
    )
  }

  return (
    <main className="auth-layout">
      <section className="auth-card">
        <div className="brand-row"><span className="brand-mark"><Database size={22} /></span><span>Supabase Manager</span></div>
        <p className="eyebrow">First-time setup</p>
        <h1>Create the administrator</h1>
        <p className="muted">This account controls every Supabase runtime on this server.</p>
        <form onSubmit={form.handleSubmit((values) => setup.mutate(values))}>
          <label>Username<input autoComplete="username" {...form.register('username')} /></label>
          <FieldError message={form.formState.errors.username?.message} />
          <label>Password<input type="password" autoComplete="new-password" minLength={12} aria-describedby="password-hint" {...form.register('password')} /></label>
          <span className="field-hint" id="password-hint">Use 12 or more characters.</span>
          <FieldError message={form.formState.errors.password?.message} />
          <label>Confirm password<input type="password" autoComplete="new-password" {...form.register('confirmPassword')} /></label>
          <FieldError message={form.formState.errors.confirmPassword?.message} />
          {setup.error && <div className="alert error">{setup.error.message}</div>}
          <button className="button primary full" disabled={setup.isPending} type="submit">{setup.isPending ? 'Creating…' : 'Create administrator'}</button>
        </form>
      </section>
    </main>
  )
}

function FieldError({ message }: { message?: string }) {
  return message ? <span className="field-error">{message}</span> : null
}
