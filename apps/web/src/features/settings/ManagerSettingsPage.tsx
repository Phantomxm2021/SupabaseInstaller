import { useQuery } from '@tanstack/react-query'
import { ShieldCheck, ServerCog, UserCircle } from 'lucide-react'
import { sessionQueryOptions } from '../../api/session'
import { Badge } from '../../components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/card'

type SafeSession = {
  username: string
  mustChangePassword: boolean
}

export function ManagerSettingsPage() {
  const session = useQuery({
    ...sessionQueryOptions(),
    retry: false,
    // Deliberately project the shared response to fields that are safe to display.
    select: (response): SafeSession => ({ username: response.username, mustChangePassword: response.mustChangePassword }),
  })

  if (session.isLoading) return <main className="page"><p className="muted">Loading settings…</p></main>
  if (session.error) return <main className="page"><div className="alert error">{session.error.message}</div></main>

  const account = session.data
  return (
    <main className="page narrow-page">
      <div className="page-heading">
        <div><p className="eyebrow">Control plane</p><h1>Manager settings</h1><p className="muted">Administrator account and safe system information.</p></div>
      </div>
      <div className="settings-grid">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><UserCircle className="size-4" /> Administrator account</CardTitle>
            <CardDescription>Your session stays in an HttpOnly cookie.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <div><span className="muted text-xs">Username</span><p className="mt-1 font-medium">{account?.username}</p></div>
            <div className="flex items-center justify-between gap-3"><span className="muted text-xs">Password status</span><Badge variant={account?.mustChangePassword ? 'destructive' : 'secondary'}>{account?.mustChangePassword ? 'Change your password' : 'Password current'}</Badge></div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><ServerCog className="size-4" /> Control-plane status</CardTitle>
            <CardDescription>Safe runtime information from this Manager instance.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            <div className="flex items-center gap-2"><ShieldCheck className="size-4 text-emerald-400" /><span>Manager API</span><Badge variant="secondary" className="ml-auto">Connected</Badge></div>
            <p className="muted text-xs">Provisioner credentials and CSRF values are never shown here.</p>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}
