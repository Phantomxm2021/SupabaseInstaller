import { useState } from 'react'
import { Trash2 } from 'lucide-react'
import type { UseFormReturn } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { OAUTH_PROVIDERS, specialFields, type OAuthProvider, type ProjectForm } from './projectSchema'

export const providerLabels: Record<string, string> = { apple: 'Apple', azure: 'Azure / Microsoft', bitbucket: 'Bitbucket', discord: 'Discord', facebook: 'Facebook', figma: 'Figma', github: 'GitHub', gitlab: 'GitLab', google: 'Google', kakao: 'Kakao', keycloak: 'Keycloak', linkedin_oidc: 'LinkedIn OIDC', notion: 'Notion', slack_oidc: 'Slack OIDC', snapchat: 'Snapchat', spotify: 'Spotify', twitch: 'Twitch', twitter: 'Twitter / X', workos: 'WorkOS', zoom: 'Zoom' }

export function OAuthProviderFields({ form, siteUrl }: { form: UseFormReturn<ProjectForm>; siteUrl: string }) {
  const [removing, setRemoving] = useState<OAuthProvider>()
  const oauth = form.watch('configuration.auth.oauth') || {}
  const getError = (path: string) => { let value: any = form.formState.errors; for (const part of path.split('.')) value = value?.[part]; return (value?.message || value?.value?.message) as string | undefined }
  const callback = `${siteUrl.replace(/\/$/, '')}/auth/v1/callback`
  const remove = () => {
    if (!removing) return
    const { [removing]: _, ...remaining } = form.getValues('configuration.auth.oauth') || {}
    form.setValue('configuration.auth.oauth', remaining, { shouldDirty: true, shouldValidate: true })
    form.setValue('preset', 'CUSTOM', { shouldDirty: true })
    setRemoving(undefined)
  }
  const enabledProviders = OAUTH_PROVIDERS.filter((provider) => oauth[provider]?.enabled)
  return <><div className="grid gap-4 md:grid-cols-2">{enabledProviders.map((provider) => { const config = oauth[provider]; const special = specialFields[provider]; const path = `configuration.auth.oauth.${provider}`; return <Card size="sm" key={provider} id={`oauth-provider-${provider}`} tabIndex={-1}><CardHeader><div className="flex items-center justify-between gap-2"><CardTitle>{providerLabels[provider]}</CardTitle><Button type="button" size="icon" variant="ghost" aria-label={`Remove ${providerLabels[provider]}`} onClick={() => setRemoving(provider)}><Trash2 /></Button></div><CardDescription>Callback is read-only and generated from Site URL.</CardDescription></CardHeader><CardContent className="space-y-3"><Field><FieldLabel htmlFor={`${provider}-client-id`}>Client ID</FieldLabel><Input id={`${provider}-client-id`} {...form.register(`${path}.clientId` as any)} aria-invalid={!!getError(`${path}.clientId`)} /><FieldError>{getError(`${path}.clientId`)}</FieldError></Field><Field><FieldLabel htmlFor={`${provider}-secret`}>Client secret</FieldLabel><Input id={`${provider}-secret`} type="password" placeholder={config.secretSet ? 'Configured — enter to replace' : ''} onChange={(event) => form.setValue(`${path}.secret` as any, event.target.value ? { action: 'replace', value: event.target.value } : { action: '' }, { shouldDirty: true, shouldValidate: true })} aria-invalid={!!getError(`${path}.secret`)} /><FieldError>{getError(`${path}.secret`)}</FieldError></Field>{special && <Field><FieldLabel htmlFor={`${provider}-${special}`}>{special}</FieldLabel><Input id={`${provider}-${special}`} {...form.register(`${path}.fields.${special}` as any)} aria-invalid={!!getError(`${path}.fields.${special}`)} /><FieldError>{getError(`${path}.fields.${special}`)}</FieldError></Field>}<Field><FieldLabel htmlFor={`${provider}-callback`}>Read-only callback URL</FieldLabel><Input id={`${provider}-callback`} readOnly value={callback} /></Field></CardContent></Card> })}</div><AlertDialog open={!!removing} onOpenChange={(open) => !open && setRemoving(undefined)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Remove {removing ? providerLabels[removing] : ''}?</AlertDialogTitle><AlertDialogDescription>This removes the provider configuration from this project.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction variant="destructive" onClick={remove}>Remove</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog></>
}
