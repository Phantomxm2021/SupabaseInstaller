import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import type { FieldValues, Path, UseFormReturn } from 'react-hook-form'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import type { SecretAction, SecretInput, UpdateSecretInput } from '../../../api/types'

export function SectionCard({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return <Card><CardHeader><CardTitle>{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader><CardContent className="space-y-5">{children}</CardContent></Card>
}

/** Reset a section only when the server advances its revision. A parent render
 * (including opening the confirmation dialog or receiving a 409) must never
 * overwrite values that the administrator is editing. */
export function useResetOnServerRevision<T extends FieldValues>(form: UseFormReturn<T>, initial: T, revision: number) {
  const lastRevision = useRef(revision)
  useEffect(() => {
    if (lastRevision.current === revision) return
    lastRevision.current = revision
    if (!form.formState.isDirty) form.reset(initial)
  }, [form, initial, revision])
}

export function Toggle({ id, label, checked, onChange, disabled, description }: { id?: string; label: string; checked: boolean; onChange: (value: boolean) => void; disabled?: boolean; description?: string }) {
  const uid = useId()
  const controlId = id ?? `toggle-${uid.replace(/[^a-zA-Z0-9]/g, '')}`
  return <div className="rounded-lg border border-border p-3"><div className="flex items-center justify-between gap-3"><label htmlFor={controlId} className="text-sm font-medium">{label}</label><Switch id={controlId} aria-label={label} checked={checked} onCheckedChange={onChange} disabled={disabled} /></div>{description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}</div>
}

export function TextField<T extends FieldValues>({ form, name, label, placeholder }: { form: UseFormReturn<T>; name: Path<T>; label: string; placeholder?: string }) {
  const uid = useId()
  const id = `field-${String(name).replace(/[^a-zA-Z0-9]+/g, '-')}-${uid.replace(/[^a-zA-Z0-9]/g, '')}`
  const error = errorAt(form.formState.errors, String(name))
  return <Field><FieldLabel htmlFor={id}>{label}</FieldLabel><Input id={id} placeholder={placeholder} {...form.register(name)} aria-invalid={Boolean(error)} aria-describedby={error ? `${id}-error` : undefined} />{error && <FieldError id={`${id}-error`}>{error}</FieldError>}</Field>
}

export function NumberField<T extends FieldValues>({ form, name, label, min, max }: { form: UseFormReturn<T>; name: Path<T>; label: string; min?: number; max?: number }) {
  const uid = useId()
  const id = `field-${String(name).replace(/[^a-zA-Z0-9]+/g, '-')}-${uid.replace(/[^a-zA-Z0-9]/g, '')}`
  const error = errorAt(form.formState.errors, String(name))
  return <Field><FieldLabel htmlFor={id}>{label}</FieldLabel><Input id={id} type="number" min={min} max={max} {...form.register(name, { valueAsNumber: true })} aria-invalid={Boolean(error)} aria-describedby={error ? `${id}-error` : undefined} />{error && <FieldError id={`${id}-error`}>{error}</FieldError>}</Field>
}

export function ReadOnlyField({ label, value, copy = false }: { label: string; value: string; copy?: boolean }) {
  const uid = useId()
  const id = `readonly-${label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-${uid.replace(/[^a-zA-Z0-9]/g, '')}`
  return <Field><FieldLabel htmlFor={id}>{label}</FieldLabel><div className="flex gap-2"><Input id={id} value={value} readOnly aria-readonly="true" />{copy && <Button type="button" variant="outline" onClick={() => void navigator.clipboard?.writeText(value)} aria-label={`Copy ${label}`}>Copy</Button>}</div></Field>
}
export function SectionSaveButton({ label, disabled }: { label: string; disabled: boolean }) { return <div className="flex justify-end"><Button type="submit" disabled={disabled}><span>Save {label}</span></Button></div> }

export function SecretEditor({ label, secret, configured, onChange }: { label: string; secret?: SecretInput; configured: boolean; onChange: (value: UpdateSecretInput) => void }) {
  const [value, setValue] = useStateMemory()
  const [visible, setVisible] = useState(false)
  const uid = useId()
  const id = `secret-${label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-${uid.replace(/[^a-zA-Z0-9]/g, '')}`
  const action: SecretAction = value ? 'replace' : ''
  return <Field><div className="flex items-center justify-between gap-2"><FieldLabel htmlFor={id}>{label}</FieldLabel>{configured && <Badge variant="outline">Configured</Badge>}</div><div className="flex gap-2"><Input id={id} type={visible ? 'text' : 'password'} value={value} placeholder={configured ? 'Configured — enter to replace' : 'Enter a secret'} autoComplete="new-password" onChange={(event) => { const next = event.target.value; setValue(next); onChange(next ? { action: 'replace', value: next } : { action: '' }) }} /><Button type="button" variant="outline" size="icon" onClick={() => setVisible((current) => !current)} aria-label={visible ? `Hide ${label}` : `Show ${label}`}>{visible ? 'Hide' : 'Show'}</Button></div>{configured && <div className="flex gap-2"><Button type="button" size="sm" variant="ghost" onClick={() => { setValue(''); onChange({ action: 'retain' }) }}>Retain</Button><Button type="button" size="sm" variant="ghost" onClick={() => { setValue(''); onChange({ action: 'remove' }) }}>Remove</Button></div>}<FieldDescription>Secrets are held in memory only and are never returned by configuration reads.</FieldDescription></Field>
}

function useStateMemory() {
  // A tiny local state helper keeps write-only values out of form defaults and query caches.
  const [value, setValue] = useState('')
  useEffect(() => () => setValue(''), [])
  return [value, setValue] as const
}
function errorAt(errors: unknown, path: string) { let current: any = errors; for (const part of path.split('.')) current = current?.[part]; return current?.message ?? current?.root?.message }
