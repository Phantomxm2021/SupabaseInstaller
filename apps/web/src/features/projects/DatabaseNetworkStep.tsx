import type { UseFormReturn } from 'react-hook-form'
import { Alert } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { ProjectForm } from './projectSchema'
import { setServiceEnabled } from './PresetStep'

type InfrastructureFormProps = { form: UseFormReturn<ProjectForm> }

export function DatabaseNetworkStep({ form }: InfrastructureFormProps) {
  return <Card><CardHeader><CardTitle>Database, pooler & network</CardTitle><CardDescription>Manager allocates host ports from zero. Unsupported pinned-runtime fields are read-only and omitted from create requests.</CardDescription></CardHeader><CardContent className="space-y-7"><DatabaseRealtimeFields form={form} /><PoolerFields form={form} /><GatewayNetworkFields form={form} /><Alert>Manager validates unique ports and derives disabled service ports during installation.</Alert></CardContent></Card>
}

export function DatabaseRealtimeFields({ form }: InfrastructureFormProps) {
  const db = form.watch('configuration.database')
  const services = form.watch('configuration.services')
  const realtime = form.watch('configuration.realtime')
  const set = (name: string, value: unknown) => { form.setValue(name as never, value as never, { shouldDirty: true, shouldValidate: true }); form.setValue('preset', 'CUSTOM', { shouldDirty: true }) }
  const error = fieldError(form)
  return <><section className="space-y-4"><h3 className="font-medium">Database</h3><FieldGroup className="grid gap-4 md:grid-cols-2"><Field><FieldLabel>Postgres version</FieldLabel><Input readOnly value={db.version} /></Field><NumberField label="Max connections" name="configuration.database.maxConnections" form={form} error={error('configuration.database.maxConnections')} /><Field><FieldLabel htmlFor="shared-buffers">Shared buffers</FieldLabel><Input id="shared-buffers" {...form.register('configuration.database.sharedBuffers')} /></Field><Field><FieldLabel>Extensions</FieldLabel><Input readOnly value="Not supported by self-hosted/v0.8.0" /><FieldDescription>Extensions are intentionally unavailable for this pinned renderer.</FieldDescription></Field></FieldGroup><Field orientation="horizontal" className="items-center justify-between rounded-lg border border-border px-3 py-2"><FieldLabel>Direct PostgreSQL port</FieldLabel><Switch aria-label="Direct PostgreSQL port" checked={services.directDb} onCheckedChange={(enabled) => setServiceEnabled(form, 'directDb', enabled)} /></Field>{services.directDb && <Field><FieldLabel>Direct database allocated port</FieldLabel><Input readOnly value={form.watch('configuration.network.directDatabasePort') || 'Allocated by Manager'} /></Field>}</section>
    <section className="space-y-4"><h3 className="font-medium">Realtime</h3><FieldGroup className="grid gap-4 md:grid-cols-2"><NumberField label="Max connections" name="configuration.realtime.maxConnections" form={form} error={error('configuration.realtime.maxConnections')} /><NumberField label="Database pool size" name="configuration.realtime.databasePoolSize" form={form} error={error('configuration.realtime.databasePoolSize')} /><Field><FieldLabel>Log level</FieldLabel><Select value={realtime.logLevel} onValueChange={(value) => set('configuration.realtime.logLevel', value)}><SelectTrigger aria-label="Realtime log level"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="debug">Debug</SelectItem><SelectItem value="info">Info</SelectItem><SelectItem value="warn">Warn</SelectItem><SelectItem value="error">Error</SelectItem></SelectContent></Select></Field></FieldGroup></section></>
}

export function PoolerFields({ form }: InfrastructureFormProps) {
  const pooler = form.watch('configuration.pooler')
  const services = form.watch('configuration.services')
  const error = fieldError(form)
  return <section className="space-y-4"><div className="flex items-center justify-between"><h3 className="font-medium">Supavisor pooler</h3><Switch aria-label="Enable Supavisor" checked={services.supavisor} onCheckedChange={(enabled) => setServiceEnabled(form, 'supavisor', enabled)} /></div><FieldGroup className="grid gap-4 md:grid-cols-2"><NumberField label="Pool size" name="configuration.pooler.poolSize" form={form} error={error('configuration.pooler.poolSize')} /><NumberField label="Maximum client connections" name="configuration.pooler.maxClientConnections" form={form} error={error('configuration.pooler.maxClientConnections')} /><ReadOnlyPort label="Transaction port" value={pooler.transactionPort} /><ReadOnlyPort label="Session port" value={pooler.sessionPort} /></FieldGroup><FieldDescription>Transaction and session ports are allocated by Manager and cannot be edited.</FieldDescription></section>
}

export function GatewayNetworkFields({ form }: InfrastructureFormProps) {
  const network = form.watch('configuration.network')
  const set = (name: string, value: unknown) => { form.setValue(name as never, value as never, { shouldDirty: true, shouldValidate: true }); form.setValue('preset', 'CUSTOM', { shouldDirty: true }) }
  return <section className="space-y-4"><FieldGroup className="grid gap-4 md:grid-cols-2"><Field><FieldLabel>Gateway</FieldLabel><Select value={network.gateway} onValueChange={(value) => set('configuration.network.gateway', value)}><SelectTrigger aria-label="Gateway"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="envoy">Envoy</SelectItem><SelectItem value="kong">Kong (advanced)</SelectItem></SelectContent></Select></Field><Field><FieldLabel>HTTPS mode</FieldLabel><Select value={network.httpsMode} onValueChange={(value) => set('configuration.network.httpsMode', value)}><SelectTrigger aria-label="HTTPS mode"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="external">External reverse proxy</SelectItem></SelectContent></Select><FieldDescription>TLS is terminated outside this manager.</FieldDescription></Field><ReadOnlyPort label="Internal gateway port" value={network.internalGatewayPort} /><ReadOnlyPort label="API port" value={network.apiPort} /><ReadOnlyPort label="Studio port" value={network.studioPort} /><ReadOnlyPort label="Direct database allocated port" value={network.directDatabasePort} /><ReadOnlyPort label="Pooler port" value={network.poolerPort} /></FieldGroup>{network.httpsMode === 'external' && <Alert>External reverse proxy mode emits upstream instructions; TLS is terminated outside this manager.</Alert>}</section>
}

function fieldError(form: UseFormReturn<ProjectForm>) { return (name: string) => { let value: any = form.formState.errors; for (const part of name.split('.')) value = value?.[part]; return value?.message as string | undefined } }
function NumberField({ label, name, form, error }: { label: string; name: string; form: UseFormReturn<ProjectForm>; error?: string }) { return <Field><FieldLabel htmlFor={name}>{label}</FieldLabel><Input id={name} type="number" {...form.register(name as never, { valueAsNumber: true })} aria-invalid={!!error} /><FieldError>{error}</FieldError></Field> }
function ReadOnlyPort({ label, value }: { label: string; value?: number }) { return <Field><FieldLabel>{label}</FieldLabel><Input readOnly value={value || 'Allocated by Manager'} /></Field> }
