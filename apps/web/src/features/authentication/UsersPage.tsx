import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { ArrowDownNarrowWide, ChevronDown, Columns3, LockKeyhole, Mail, RefreshCw, Search, UserPlus, UserRound, X } from 'lucide-react'
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
  const body = useMemo(() => {
    if (users.isLoading) return <tr><td colSpan={8} className="auth-users-empty">Loading users…</td></tr>
    if (users.error) return <tr><td colSpan={8} className="auth-users-empty">Unable to load users.</td></tr>
    if (rows.length === 0) return <tr><td colSpan={8} className="auth-users-empty">No users found</td></tr>
    return rows.map((user) => <UserRow key={user.id} user={user} />)
  }, [rows, users.error, users.isLoading])

  return <main className="page auth-page auth-users-page">
    <header className="auth-users-heading"><h1>Users</h1></header>
    <div className="auth-users-toolbar">
      <div className="auth-users-search">
        <Search aria-hidden="true" />
        <button type="button" className="auth-users-search-type">Email address <ChevronDown /></button>
        <Input aria-label="Search users by email" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search by email" />
      </div>
      <span className="auth-users-toolbar-divider" />
      <button type="button" className="auth-users-columns"><Columns3 aria-hidden="true" />All columns <ChevronDown /></button>
      <button type="button" className="auth-users-sorted" aria-disabled="true"><ArrowDownNarrowWide aria-hidden="true" />Sorted by user ID</button>
      <span className="auth-users-toolbar-spacer" />
      <Button type="button" size="icon-lg" variant="outline" aria-label="Refresh users" className="auth-users-refresh" onClick={() => void users.refetch()}><RefreshCw /></Button>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button type="button" aria-label="Add user" size="lg" className="auth-users-add" />}>
          Add user <ChevronDown />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" sideOffset={8} className="auth-add-user-menu">
          <DropdownMenuItem onClick={() => setInviteOpen(true)}><Mail /><span>Send invitation</span><kbd>I then I</kbd></DropdownMenuItem>
          <DropdownMenuItem onClick={() => setCreateOpen(true)}><UserPlus /><span>Create new user</span><kbd>I then U</kbd></DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <section className="auth-users-table-card">
      <div className="auth-users-table-wrap">
        <table className="auth-users-table">
          <colgroup><col className="auth-users-select-col" /><col className="auth-users-uid-col" /><col className="auth-users-name-col" /><col className="auth-users-email-col" /><col className="auth-users-phone-col" /><col className="auth-users-providers-col" /><col className="auth-users-type-col" /><col className="auth-users-created-col" /></colgroup>
          <thead><tr><th /><th>UID</th><th>Display name</th><th>Email</th><th>Phone</th><th>Providers</th><th>Provider type</th><th>Created at</th></tr></thead>
          <tbody>{body}</tbody>
        </table>
      </div>
      <footer>Total: {rows.length} user{rows.length === 1 ? '' : 's'}</footer>
    </section>
    <InviteUserDialog open={inviteOpen} onOpenChange={setInviteOpen} pending={invite.isPending} onInvite={(email) => invite.mutate(email)} />
    <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} pending={create.isPending} onCreate={(input) => create.mutate(input)} />
  </main>
}

function UserRow({ user }: { user: AuthUser }) {
  const displayName = String(user.user_metadata?.display_name ?? user.user_metadata?.name ?? '')
  const providers = [...new Set(user.identities?.map((identity) => identity.provider) ?? [])]
  const social = providers.some((provider) => provider !== 'email' && provider !== 'phone')
  return <tr>
    <td className="auth-users-select"><input aria-label={`Select ${user.email || user.id}`} type="checkbox" /><span className="auth-user-avatar">{displayName ? displayName.slice(0, 1).toUpperCase() : <UserRound />}</span></td>
    <td className="auth-users-id">{user.id}</td>
    <td>{displayName || '—'}</td>
    <td>{user.email || '—'}</td>
    <td>{user.phone || '—'}</td>
    <td><ProviderList providers={providers} fallback={user.email ? 'email' : ''} /></td>
    <td>{social ? 'Social' : '—'}</td>
    <td>{formatCreatedAt(user.created_at)}</td>
  </tr>
}

function formatCreatedAt(value: string | undefined) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const weekday = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'][date.getDay()]
  const month = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'][date.getMonth()]
  const offset = -date.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const offsetHours = String(Math.floor(Math.abs(offset) / 60)).padStart(2, '0')
  const offsetMinutes = String(Math.abs(offset) % 60).padStart(2, '0')
  return `${weekday} ${String(date.getDate()).padStart(2, '0')} ${month} ${date.getFullYear()} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')} GMT${sign}${offsetHours}${offsetMinutes}`
}

function ProviderList({ providers, fallback }: { providers: string[]; fallback: string }) {
  const names = providers.length ? providers : fallback ? [fallback] : []
  if (names.length === 0) return <>—</>
  return <span className="auth-provider-list">{names.map((provider) => <span key={provider} className="auth-provider"><span className={`auth-provider-icon auth-provider-${provider.replaceAll('_', '-')}`}>{provider === 'email' ? <Mail /> : provider.slice(0, 1).toUpperCase()}</span>{providerLabel(provider)}</span>)}</span>
}

function providerLabel(provider: string) {
  if (provider === 'email') return 'Email'
  return provider.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function InviteUserDialog({ open, onOpenChange, pending, onInvite }: { open: boolean; onOpenChange: (open: boolean) => void; pending: boolean; onInvite: (email: string) => void }) {
  const [email, setEmail] = useState('')
  return <AlertDialog open={open} onOpenChange={onOpenChange}>
    <AlertDialogContent className="auth-user-dialog auth-invite-dialog sm:max-w-[24rem]" aria-describedby={undefined}>
      <AlertDialogHeader className="auth-user-dialog-header"><AlertDialogTitle>Invite a new user</AlertDialogTitle><AlertDialogCancel variant="ghost" size="icon" className="auth-user-dialog-close" aria-label="Close"><X /></AlertDialogCancel></AlertDialogHeader>
      <form className="auth-invite-form" onSubmit={(event) => { event.preventDefault(); onInvite(email) }}>
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
    <AlertDialogContent className="auth-user-dialog auth-create-dialog sm:max-w-[24rem]" aria-describedby={undefined}>
      <AlertDialogHeader className="auth-user-dialog-header"><AlertDialogTitle>Create a new user</AlertDialogTitle><AlertDialogCancel variant="ghost" size="icon" className="auth-user-dialog-close" aria-label="Close"><X /></AlertDialogCancel></AlertDialogHeader>
      <form className="auth-create-user-form" onSubmit={(event) => { event.preventDefault(); onCreate({ email, password, email_confirm: emailConfirm }) }}>
        <label className="auth-user-field">Email address<span className="auth-user-input"><Mail /><Input value={email} onChange={(event) => setEmail(event.target.value)} type="email" placeholder="user@example.com" autoFocus required /></span></label>
        <label className="auth-user-field">User Password<span className="auth-user-input"><LockKeyhole /><Input value={password} onChange={(event) => setPassword(event.target.value)} type="password" required minLength={6} /></span></label>
        <label className="auth-auto-confirm"><Checkbox checked={emailConfirm} onCheckedChange={setEmailConfirm} />Auto confirm user?</label>
        <p className="muted">A confirmation email will not be sent when creating a user via this form.</p>
        <Button type="submit" className="w-full" size="lg" disabled={pending}>{pending ? 'Creating…' : 'Create user'}</Button>
      </form>
    </AlertDialogContent>
  </AlertDialog>
}
