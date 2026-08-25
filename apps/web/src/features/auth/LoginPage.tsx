import { useMutation } from '@tanstack/react-query'
import { Database } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { useNavigate } from 'react-router-dom'
import { apiFetch, setCSRFToken } from '../../api/client'

interface LoginForm { username: string; password: string }

export function LoginPage() {
  const navigate = useNavigate()
  const form = useForm<LoginForm>({ defaultValues: { username: '', password: '' } })
  const login = useMutation({
    mutationFn: (values: LoginForm) => apiFetch<{ csrfToken: string }>('/api/session', { method: 'POST', body: JSON.stringify(values) }),
    onSuccess: (session) => { setCSRFToken(session.csrfToken); navigate('/projects', { replace: true }) },
  })
  return (
    <main className="auth-layout">
      <section className="auth-card">
        <div className="brand-row"><span className="brand-mark"><Database size={22} /></span><span>Supabase Manager</span></div>
        <p className="eyebrow">Server administration</p>
        <h1>Welcome back</h1>
        <p className="muted">Sign in to manage your self-hosted projects.</p>
        <form onSubmit={form.handleSubmit((values) => login.mutate(values))}>
          <label>Username<input autoComplete="username" required {...form.register('username')} /></label>
          <label>Password<input type="password" autoComplete="current-password" required {...form.register('password')} /></label>
          {login.error && <div className="alert error">{login.error.message}</div>}
          <button className="button primary full" disabled={login.isPending}>{login.isPending ? 'Signing in…' : 'Sign in'}</button>
        </form>
      </section>
    </main>
  )
}
