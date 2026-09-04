import { useEffect, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Eye, EyeOff, RotateCcw, Trash2, X } from 'lucide-react'
import { useForm, type Resolver } from 'react-hook-form'
import type { FunctionVariable, FunctionsConfig, UpdateSecretInput } from '../../../api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FieldError } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { functionsSchema } from './schema'
import { errorAt, SectionCard, SectionSaveButton, Toggle, useResetOnServerRevision, type SectionSave } from './fields'

function VariableValueInput({ label, configured, disabled, error, onChange }: {
  label: string; configured: boolean; disabled: boolean; error?: string
  onChange: (value: UpdateSecretInput) => void
}) {
  const [value, setValue] = useState('')
  const [visible, setVisible] = useState(false)
  useEffect(() => () => setValue(''), [])
  return <div className="min-w-[240px]"><div className="flex items-center gap-2">
    <Input aria-label={label} type={visible ? 'text' : 'password'} value={value} disabled={disabled}
      placeholder={configured ? 'Configured — enter to replace' : 'Enter a value'} autoComplete="new-password"
      aria-invalid={Boolean(error)} onChange={(event) => { const next = event.target.value; setValue(next); onChange(next ? { action: 'replace', value: next } : { action: '' }) }} />
    <Button type="button" variant="outline" size="icon" disabled={disabled} aria-label={`${visible ? 'Hide' : 'Show'} ${label}`} onClick={() => setVisible((current) => !current)}>{visible ? <EyeOff /> : <Eye />}</Button>
  </div>{error && <FieldError className="mt-1">{error}</FieldError>}</div>
}

export function FunctionsSection({ initial, revision, enabled, onSave }: {
  initial: FunctionsConfig; revision: number; enabled: boolean; onSave: SectionSave<FunctionsConfig>
}) {
  const form = useForm<FunctionsConfig>({ resolver: zodResolver(functionsSchema) as Resolver<FunctionsConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const functions = form.watch()
  const variables = functions.variables ?? []
  const updateVariables = (next: FunctionVariable[]) => form.setValue('variables', next, { shouldDirty: true, shouldValidate: true })
  const updateVariable = (index: number, patch: Partial<FunctionVariable>) => { const next = [...variables]; next[index] = { ...next[index], ...patch }; updateVariables(next) }
  const setError = (name: string, message: string) => form.setError(name as never, { type: 'server', message })

  return <form id="configuration-functions-form" onSubmit={form.handleSubmit((value) => onSave({
    value: { ...value, variables: value.variables.map((item) => item.value.action === 'replace' ? { ...item, valueSet: true } : item) },
    dirty: form.formState.dirtyFields,
    setError,
  }))} className="space-y-5">
    <SectionCard title="Functions" description="Environment values are encrypted and rendered into the Functions runtime. Deleting a configured variable explicitly removes its secret.">
      <p className="text-sm text-muted-foreground">Functions service is currently <strong>{enabled ? 'enabled' : 'disabled'}</strong>; enablement is owned by Services.</p>
      <Toggle id="functions-jwt" label="Default JWT verification" checked={functions.defaultJwtVerification} onChange={(value) => form.setValue('defaultJwtVerification', value, { shouldDirty: true, shouldValidate: true })} />
      <div className="space-y-3">
        <div className="flex items-center justify-between gap-3"><div><h3 className="text-sm font-semibold">Environment variables</h3><p className="text-xs text-muted-foreground">Values are write-only. Enter a value only when adding or replacing a variable.</p></div>
          <Button type="button" variant="outline" onClick={() => updateVariables([...variables, { name: '', valueSet: false, value: { action: '' } }])}>Add variable</Button></div>
        <div className="overflow-hidden rounded-lg border border-border"><Table><TableHeader><TableRow>
          <TableHead className="w-[28%]">Name</TableHead><TableHead>Value</TableHead><TableHead className="w-[150px]">Status</TableHead><TableHead className="w-[70px] text-right">Actions</TableHead>
        </TableRow></TableHeader><TableBody>
          {variables.length === 0 && <TableRow><TableCell colSpan={4} className="h-20 text-center text-muted-foreground">No environment variables configured.</TableCell></TableRow>}
          {variables.map((item, index) => {
            const pendingRemoval = item.value.action === 'remove'
            const replacementPending = item.value.action === 'replace'
            const nameLabel = item.name || String(index + 1)
            const nameError = errorAt(form.formState.errors, `variables.${index}.name`)
            const valueError = errorAt(form.formState.errors, `variables.${index}.value`)
            return <TableRow key={index} className={pendingRemoval ? 'bg-destructive/5' : undefined}>
              <TableCell className="align-top whitespace-normal">{item.valueSet ? <code className="break-all text-xs font-medium">{item.name}</code> : <div><Input aria-label="Variable name" value={item.name} placeholder="VARIABLE_NAME" aria-invalid={Boolean(nameError)} onChange={(event) => updateVariable(index, { name: event.target.value.toUpperCase() })} />{nameError && <FieldError className="mt-1">{nameError}</FieldError>}</div>}</TableCell>
              <TableCell className="align-top whitespace-normal"><VariableValueInput label={`Value for ${item.name || `variable ${index + 1}`}`} configured={item.valueSet} disabled={pendingRemoval} error={valueError} onChange={(value) => updateVariable(index, { value })} /></TableCell>
              <TableCell className="align-top">{pendingRemoval ? <Badge variant="destructive">Pending removal</Badge> : replacementPending ? <Badge variant="secondary">Replacement pending</Badge> : item.valueSet ? <Badge variant="outline">Configured</Badge> : <Badge variant="secondary">New</Badge>}</TableCell>
              <TableCell className="align-top text-right">{pendingRemoval ? <Button type="button" variant="ghost" size="icon" aria-label={`Undo removal ${nameLabel}`} onClick={() => updateVariable(index, { value: { action: '' } })}><RotateCcw /></Button> : <Button type="button" variant="ghost" size="icon" aria-label={`${item.valueSet ? 'Remove variable' : 'Cancel variable'} ${nameLabel}`} onClick={() => item.valueSet ? updateVariable(index, { value: { action: 'remove' } }) : updateVariables(variables.filter((_, current) => current !== index))}>{item.valueSet ? <Trash2 /> : <X />}</Button>}</TableCell>
            </TableRow>
          })}
        </TableBody></Table></div>
      </div>
    </SectionCard>
    <SectionSaveButton label="Functions" disabled={!form.formState.isDirty} />
  </form>
}
