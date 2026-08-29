import { useMutation } from '@tanstack/react-query'
import { Database } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { useNavigate } from 'react-router-dom'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { apiFetch, setCSRFToken } from '../../api/client'
interface LoginForm { username: string; password: string }
export function LoginPage() {
  const navigate = useNavigate(); const form = useForm<LoginForm>({ defaultValues: { username: '', password: '' } }); const login = useMutation({ mutationFn: (values: LoginForm) => apiFetch<{ csrfToken: string }>('/api/session', { method: 'POST', body: JSON.stringify(values) }), onSuccess: (session) => { setCSRFToken(session.csrfToken); navigate('/projects', { replace: true }) } })
  return <main className="auth-layout"><Card className="w-full max-w-md"><CardHeader><div className="mb-4 flex items-center gap-2 font-semibold"><span className="inline-flex size-9 items-center justify-center rounded-lg border border-primary/25 bg-primary/10 text-primary"><Database className="size-5" /></span>Supabase Manager</div><p className="eyebrow">Server administration</p><CardTitle>Welcome back</CardTitle><CardDescription>Sign in to manage your self-hosted servers.</CardDescription></CardHeader><CardContent><form onSubmit={form.handleSubmit((values) => login.mutate(values))} className="space-y-4"><Field><FieldLabel htmlFor="login-username">Username</FieldLabel><Input id="login-username" autoComplete="username" required aria-describedby="login-username-description" {...form.register('username')} /><FieldDescription id="login-username-description">Your Manager administrator username.</FieldDescription></Field><Field><FieldLabel htmlFor="login-password">Password</FieldLabel><Input id="login-password" type="password" autoComplete="current-password" required aria-describedby="login-password-description" {...form.register('password')} /><FieldDescription id="login-password-description">Your administrator credentials are used only for this session.</FieldDescription></Field>{login.error && <Alert variant="destructive">{login.error.message}</Alert>}<Button type="submit" className="w-full" disabled={login.isPending}>{login.isPending ? 'Signing in…' : 'Sign in'}</Button></form></CardContent></Card></main>
}
