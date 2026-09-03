import { useEffect, useState } from 'react'
import { AlertTriangle, Copy, Eye, EyeOff } from 'lucide-react'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SectionCard, ReadOnlyField } from './fields'
import { APIError, migrateAuthKeys, revealSecret, rotateApiKeys, rotateSigningKeys } from '../../../api/client'

const revealKinds = [
  { kind: 'anonKey', label: 'Anon Key' }, { kind: 'serviceRoleKey', label: 'Service Role Key' },
  { kind: 'jwtSecret', label: 'JWT Secret' }, { kind: 'databasePassword', label: 'Database Password' },
  { kind: 'publishable-api-key', label: 'Publishable API Key' }, { kind: 'secret-api-key', label: 'Secret API Key' },
] as const
type RevealKind = typeof revealKinds[number]['kind']
type Props = { projectId: string; projectUrl: string; projectName?: string; anonKey?: string }

export function SecretsSection({ projectId, projectUrl, projectName = projectId }: Props) {
  const [values, setValues] = useState<Partial<Record<RevealKind, string>>>({})
  const [open, setOpen] = useState(false); const [password, setPassword] = useState(''); const [confirmation, setConfirmation] = useState(''); const [error, setError] = useState('');
  const [busy, setBusy] = useState<RevealKind | 'migrate' | 'rotate-api' | 'rotate-signing'>(); const [operationId, setOperationId] = useState('')
  useEffect(() => () => setValues({}), [])
  const reveal = async (kind: RevealKind) => { setBusy(kind); setError(''); try { const result = await revealSecret(projectId, kind, password); setValues((current) => ({ ...current, [kind]: result.value })); window.setTimeout(() => setValues((current) => { const next = { ...current }; delete next[kind]; return next }), 60_000) } catch (caught) { setError(caught instanceof APIError ? caught.message : 'Unable to reveal secret') } finally { setBusy(undefined) } }
  const runOperation = async (kind: 'migrate' | 'rotate-api' | 'rotate-signing') => { setBusy(kind); setError(''); try { const input = kind === 'rotate-signing' ? { password, confirmProjectName: confirmation } : { password }; const result = kind === 'migrate' ? await migrateAuthKeys(projectId, input) : kind === 'rotate-api' ? await rotateApiKeys(projectId, input) : await rotateSigningKeys(projectId, input); setOperationId(result.operationId); setPassword(''); setConfirmation('') } catch (caught) { setError(caught instanceof APIError ? caught.message : 'Unable to queue auth-key operation') } finally { setBusy(undefined) } }
  const copy = async (kind: RevealKind) => { if (values[kind]) await navigator.clipboard?.writeText(values[kind] as string) }
  const signingReady = Boolean(password) && confirmation === projectName
  return <SectionCard title="API & Secrets" description="Project URL is public. Secret values require recent administrator authentication and never appear in configuration responses.">
    <ReadOnlyField label="Project URL" value={projectUrl || 'Unavailable'} copy={Boolean(projectUrl)} />
    <div className="rounded-lg border border-border p-4 space-y-4"><div className="flex items-center gap-2"><AlertTriangle className="size-4 text-amber-500" /><strong className="text-sm">Public and sensitive values</strong></div><p className="text-sm text-muted-foreground">Legacy keys remain available during migration. Signing internals and role credentials stay server-side.</p><Button type="button" variant="outline" onClick={() => { setOpen((current) => !current); setPassword(''); setConfirmation(''); setError('') }}><Eye className="size-4" />{open ? 'Hide auth-key controls' : 'Reveal API keys and secrets'}</Button>
      {open && <div className="space-y-4"><div><label htmlFor="reveal-password" className="text-sm font-medium">Administrator password</label><Input id="reveal-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></div>{error && <Alert variant="destructive">{error}</Alert>}{operationId && <Alert>Auth-key operation <code>{operationId}</code> queued. Runtime reconciliation is in progress.</Alert>}
        <div className="rounded-md border border-border p-3 space-y-3"><strong className="text-sm">Auth-key migration and rotation</strong><p className="text-xs text-muted-foreground">Migration and rotations require password confirmation. Opaque API key rotation preserves signing material.</p><div className="flex flex-wrap gap-2"><Button type="button" size="sm" disabled={!password || Boolean(busy)} onClick={() => void runOperation('migrate')}>Migrate auth keys</Button><Button type="button" size="sm" variant="outline" disabled={!password || Boolean(busy)} onClick={() => void runOperation('rotate-api')}>Rotate opaque API keys</Button></div><p className="text-xs text-amber-600">Signing replacement invalidates all ES256 sessions and should be scheduled in a maintenance window.</p><label htmlFor="project-name-confirmation" className="text-sm font-medium">Type the project name to confirm signing replacement</label><Input id="project-name-confirmation" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} placeholder={projectName} /><Button type="button" size="sm" variant="destructive" disabled={!signingReady || Boolean(busy)} onClick={() => void runOperation('rotate-signing')}>Rotate signing keys</Button></div>
        <div className="space-y-3">{revealKinds.map(({ kind, label }) => <div key={kind} className="flex items-center justify-between gap-2"><span className="text-sm">{label}</span>{values[kind] ? <><code className="max-w-[260px] truncate">{values[kind]}</code><Button type="button" size="icon" variant="outline" aria-label={`Copy ${label}`} onClick={() => void copy(kind)}><Copy /></Button><Button type="button" size="icon" variant="outline" aria-label={`Hide ${label}`} onClick={() => setValues((current) => { const next = { ...current }; delete next[kind]; return next })}><EyeOff /></Button></> : <Button type="button" size="sm" variant="outline" disabled={!password || Boolean(busy)} onClick={() => void reveal(kind)}>{busy === kind ? 'Revealing…' : `Reveal ${label}`}</Button>}</div>)}</div>
      </div>}
    </div>
  </SectionCard>
}
