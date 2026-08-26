import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { ChevronDown, Mail, RefreshCw, Search, UserPlus, UserRound } from 'lucide-react'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { apiFetch } from '../../api/client'
import type { AuthUser } from '../../api/types'
import { useAuthenticationWorkspace } from './AuthenticationWorkspace'

type CreateUserInput = { email: string; password: string; email_confirm: boolean }

export function UsersPage() {
  const { projectId } = useAuthenticationWorkspace()
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [inviteOpen, setInviteOpen] = useState(false)
  const queryClient = useQueryClient()
  const users = useQuery({
    queryKey: ['auth-users', projectId, search],
    queryFn: () => apiFetch<{ users: AuthUser[] }>(`/api/projects/${projectId}/auth/users?search=${encodeURIComponent(search)}`),
    retry: false,
  })
  const create = useMutation({
    mutationFn: (input: CreateUserInput) => apiFetch<AuthUser>(`/api/projects/${projectId}/auth/users`, { method: 'POST', body: JSON.stringify(input) }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['auth-users', projectId] })
      setCreateOpen(false)
    },
  })
  const invite = useMutation({
    mutationFn: (email: string) => apiFetch<AuthUser>(`/api/projects/${projectId}/auth/users/invite`, { method: 'POST', body: JSON.stringify({ email }) }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['auth-users', projectId] })
      setInviteOpen(false)
    },
  })
  const rows = users.data?.users ?? []
  const content = useMemo(() => {
    if (users.isLoading) return <tr><td colSpan={8} className="auth-data-empty">Loading users…</td></tr>
    if (users.error) return <tr><td colSpan={8} className="auth-data-empty">Unable to load users.</td></tr>
    if (rows.length === 0) return <tr><td colSpan={8} className="auth-data-empty">No users found</td></tr>
    return rows.map((user) => <tr key={user.id}>
      <td><input aria-label={`Select ${user.email || user.id}`} type="checkbox" /></td>
      <td><span className="auth-user-avatar"><UserRound /></span></td>
      <td className="auth-user-id">{user.id}</td>
      <td>{String(user.user_metadata?.display_name ?? user.user_metadata?.name ?? '—')}</td>
      <td>{user.email || '—'}</td>
      <td>{user.phone || '—'}</td>
      <td>{providerNames(user)}</td>
      <td>{providerType(user)}</td>
    </tr>)
  }, [rows, users.error, users.isLoading])

  return <main className="page auth-page auth-users-page">
    <header className="page-heading"><div><h1>Users</h1></div></header>
    <div className="auth-data-toolbar">
      <div className="auth-search"><Search aria-hidden="true" /><Input aria-label="Search users by email" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search by email" /></div>
      <Button type="button" variant="outline" className="auth-filter-button">All columns</Button>
      <Button type="button" variant="outline" className="auth-filter-button">Sorted by user ID</Button>
      <span className="auth-toolbar-spacer" />
      <Button type="button" size="icon" variant="outline" aria-label="Refresh users" onClick={() => void users.refetch()}><RefreshCw /></Button>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button type="button" aria-label="Add user" />}>
          Add user <ChevronDown />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="auth-add-user-menu">
          <DropdownMenuItem onClick={() => setInviteOpen(true)}><Mail />Send invitation</DropdownMenuItem>
          <DropdownMenuItem onClick={() => setCreateOpen(true)}><UserPlus />Create new user</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <section className="auth-data-card">
      <div className="auth-data-table-wrap"><table className="auth-data-table"><thead><tr><th /><th /><th>UID</th><th>Display name</th><th>Email</th><th>Phone</th><th>Providers</th><th>Provider type</th></tr></thead><tbody>{content}</tbody></table></div>
      <footer>Total: {rows.length} user{rows.length === 1 ? '' : 's'}</footer>
    </section>
    <InviteUserDialog open={inviteOpen} onOpenChange={setInviteOpen} pending={invite.isPending} onInvite={(email) => invite.mutate(email)} />
    <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} pending={create.isPending} onCreate={(input) => create.mutate(input)} />
  </main>
}

function providerNames(user: AuthUser) {
  const providers = [...new Set(user.identities?.map((identity) => identity.provider) ?? [])]
  return providers.length ? providers.join(', ') : user.email ? 'Email' : '—'
}

function providerType(user: AuthUser) {
  return user.identities?.some((identity) => identity.provider !== 'email' && identity.provider !== 'phone') ? 'Social' : '—'
}

function InviteUserDialog({ open, onOpenChange, pending, onInvite }: { open: boolean; onOpenChange: (open: boolean) => void; pending: boolean; onInvite: (email: string) => void }) {
  const [email, setEmail] = useState('')
  return <AlertDialog open={open} onOpenChange={onOpenChange}>
    <AlertDialogContent className="auth-user-dialog sm:max-w-[48rem]" aria-describedby={undefined}>
      <AlertDialogHeader className="auth-user-dialog-header"><AlertDialogTitle>Invite a new user</AlertDialogTitle></AlertDialogHeader>
      <form onSubmit={(event) => { event.preventDefault(); onInvite(email) }}>
        <label className="auth-user-field">User email<Input value={email} onChange={(event) => setEmail(event.target.value)} type="email" autoFocus required /></label>
        <AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction type="submit" disabled={pending}>{pending ? 'Inviting…' : 'Invite user'}</AlertDialogAction></AlertDialogFooter>
      </form>
    </AlertDialogContent>
  </AlertDialog>
}

function CreateUserDialog({ open, onOpenChange, pending, onCreate }: { open: boolean; onOpenChange: (open: boolean) => void; pending: boolean; onCreate: (input: CreateUserInput) => void }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [emailConfirm, setEmailConfirm] = useState(true)
  return <AlertDialog open={open} onOpenChange={onOpenChange}>
    <AlertDialogContent className="auth-user-dialog sm:max-w-[48rem]" aria-describedby={undefined}>
      <AlertDialogHeader className="auth-user-dialog-header"><AlertDialogTitle>Create a new user</AlertDialogTitle></AlertDialogHeader>
      <form className="auth-create-user-form" onSubmit={(event) => { event.preventDefault(); onCreate({ email, password, email_confirm: emailConfirm }) }}>
        <label className="auth-user-field">Email address<Input value={email} onChange={(event) => setEmail(event.target.value)} type="email" placeholder="user@example.com" autoFocus required /></label>
        <label className="auth-user-field">User Password<Input value={password} onChange={(event) => setPassword(event.target.value)} type="password" required minLength={6} /></label>
        <label className="auth-auto-confirm"><Checkbox checked={emailConfirm} onCheckedChange={setEmailConfirm} />Auto confirm user?</label>
        <p className="muted">A confirmation email will not be sent when creating a user via this form.</p>
        <Button type="submit" className="w-full" size="lg" disabled={pending}>{pending ? 'Creating…' : 'Create user'}</Button>
      </form>
    </AlertDialogContent>
  </AlertDialog>
}
