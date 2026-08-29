import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { BookOpen, ChevronDown, Info, Plus, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { APIError, apiFetch } from '../../api/client'
import type { OAuthApp } from '../../api/types'
import { useAuthenticationWorkspace } from './AuthenticationWorkspace'

type CreateOAuthAppInput = { name: string; redirect_uris: string[]; client_type: string; token_endpoint_auth_method: string }

export function OAuthAppsPage() {
  const { projectId } = useAuthenticationWorkspace()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const queryClient = useQueryClient()
  const apps = useQuery({
    queryKey: ['oauth-apps', projectId],
    queryFn: () => apiFetch<{ clients: OAuthApp[] }>(`/api/projects/${projectId}/auth/oauth-apps`),
    retry: false,
  })
  const create = useMutation({
    mutationFn: (input: CreateOAuthAppInput) => apiFetch<OAuthApp>(`/api/projects/${projectId}/auth/oauth-apps`, { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['oauth-apps', projectId] })
      setOpen(false)
    },
  })
  const disabled = apps.error instanceof APIError && apps.error.code === 'OAUTH_SERVER_DISABLED'
  const rows = (apps.data?.clients ?? []).filter((app) => `${app.name} ${app.client_id}`.toLowerCase().includes(search.toLowerCase()))
  const body = apps.isLoading
    ? <tr><td colSpan={5} className="auth-oauth-empty">Loading OAuth apps…</td></tr>
    : !disabled && apps.error
      ? <tr><td colSpan={5} className="auth-oauth-empty">Unable to load OAuth apps.</td></tr>
      : rows.length === 0
        ? <tr><td colSpan={5} className="auth-oauth-empty">No OAuth apps found</td></tr>
        : rows.map((app) => <tr key={app.client_id}><td>{app.name}</td><td className="auth-oauth-client-id">{app.client_id}</td><td>{app.client_type}</td><td>Manual</td><td>{app.created_at ? new Date(app.created_at).toLocaleDateString() : '—'}</td></tr>)

  return <main className="page auth-page auth-oauth-apps-page">
    <header className="auth-oauth-heading"><h1>OAuth Apps</h1><a className="auth-docs-link" href="https://supabase.com/docs/guides/auth/oauth-server" target="_blank" rel="noreferrer"><BookOpen />Docs</a></header>
    {disabled && <section className="auth-oauth-disabled"><Info aria-hidden="true" /><div><strong>OAuth Server is disabled</strong><p>Enable OAuth Server to make your server act as an identity provider for third-party applications.</p></div><Button variant="outline" type="button" disabled>OAuth Server Settings</Button></section>}
    <div className="auth-oauth-toolbar">
      <div className="auth-oauth-search"><Search aria-hidden="true" /><Input aria-label="Search OAuth apps" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search OAuth apps" /></div>
      <button type="button" className="auth-oauth-filter">Registration Type <ChevronDown /></button>
      <button type="button" className="auth-oauth-filter">Client Type <ChevronDown /></button>
      <span className="auth-oauth-toolbar-spacer" />
      <Button type="button" disabled={disabled} onClick={() => setOpen(true)}><Plus />New OAuth App</Button>
    </div>
    <section className="auth-oauth-table-card"><div className="auth-oauth-table-wrap"><table className="auth-oauth-table"><thead><tr><th>Name <span>↑</span></th><th>Client ID</th><th>Client type <span>↕</span></th><th>Registration type <span>↕</span></th><th>Created <span>↕</span></th></tr></thead><tbody>{body}</tbody></table></div></section>
    <OAuthAppSheet open={open} onOpenChange={setOpen} pending={create.isPending} onSubmit={(input) => create.mutate(input)} />
  </main>
}

function OAuthAppSheet({ open, onOpenChange, pending, onSubmit }: { open: boolean; onOpenChange: (open: boolean) => void; pending: boolean; onSubmit: (value: CreateOAuthAppInput) => void }) {
  const [name, setName] = useState('')
  const [redirect, setRedirect] = useState('')
  const [clientType, setClientType] = useState('confidential')
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent className="authentication-provider-sheet w-full overflow-y-auto sm:max-w-[560px]"><SheetHeader><SheetTitle>New OAuth App</SheetTitle></SheetHeader><form className="auth-admin-sheet-form" onSubmit={(event) => { event.preventDefault(); onSubmit({ name, redirect_uris: [redirect], client_type: clientType, token_endpoint_auth_method: clientType === 'public' ? 'none' : 'client_secret_basic' }) }}><label>Client name<Input value={name} onChange={(event) => setName(event.target.value)} required /></label><label>Redirect URI<Input type="url" value={redirect} onChange={(event) => setRedirect(event.target.value)} required placeholder="https://app.example.com/callback" /></label><label>Client type<select value={clientType} onChange={(event) => setClientType(event.target.value)}><option value="confidential">Confidential</option><option value="public">Public</option></select></label><div><Button type="submit" disabled={pending}>{pending ? 'Creating…' : 'Create OAuth App'}</Button></div></form></SheetContent></Sheet>
}
