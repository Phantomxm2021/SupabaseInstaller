import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import type { GeneralConfig, OAuthProviderConfig } from '../../../api/types'
import { OAUTH_PROVIDERS, specialFields } from '../../projects/projectSchema'
import { oauthProviderSchema } from './schema'
import { SectionCard, TextField, SecretEditor, Toggle, ReadOnlyField, SectionSaveButton, useResetOnServerRevision, errorAt, type SectionSave } from './fields'

const labels: Record<string, string> = { apple: 'Apple', azure: 'Azure / Microsoft', bitbucket: 'Bitbucket', discord: 'Discord', facebook: 'Facebook', figma: 'Figma', github: 'GitHub', gitlab: 'GitLab', google: 'Google', kakao: 'Kakao', keycloak: 'Keycloak', linkedin_oidc: 'LinkedIn OIDC', notion: 'Notion', slack_oidc: 'Slack OIDC', snapchat: 'Snapchat', spotify: 'Spotify', twitch: 'Twitch', twitter: 'Twitter / X', workos: 'WorkOS', zoom: 'Zoom' }
const emptyProvider = (): OAuthProviderConfig => ({ enabled: false, clientId: '', secretSet: false, secret: { action: '' }, fields: {} })
type OAuthSave = (provider: string, input: Parameters<SectionSave<OAuthProviderConfig>>[0]) => void

export function OAuthSection({ initial, revision, general, onSave }: { initial: Record<string, OAuthProviderConfig>; revision: number; general: GeneralConfig; onSave: OAuthSave }) {
  const callback = general.domain ? `https://${general.domain}/auth/v1/callback` : 'Set Domain to generate callback'
  return <SectionCard title="OAuth providers" description="Each provider is saved independently. Callback URLs are generated from the project public URL and never contain query parameters."><div className="grid gap-4 md:grid-cols-2">{OAUTH_PROVIDERS.map((provider) => <ProviderForm key={provider} provider={provider} initial={initial[provider] ?? emptyProvider()} revision={revision} callback={callback} onSave={onSave} />)}</div></SectionCard>
}

function ProviderForm({ provider, initial, revision, callback, onSave }: { provider: string; initial: OAuthProviderConfig; revision: number; callback: string; onSave: OAuthSave }) {
  const form = useForm<OAuthProviderConfig>({ resolver: zodResolver(oauthProviderSchema(provider)) as Resolver<OAuthProviderConfig>, defaultValues: initial })
  useResetOnServerRevision(form, initial, revision)
  const value = form.watch(); const special = specialFields[provider]
  return <form id={`configuration-oauth-${provider}-form`} onSubmit={form.handleSubmit((next) => onSave(provider, { value: next, dirty: form.formState.dirtyFields, setError: (name, message) => form.setError(name as never, { type: 'server', message }) }))} className="space-y-3 rounded-lg border border-border p-4"><div className="flex items-center justify-between gap-2"><h3 className="font-medium">{labels[provider] ?? provider}</h3><Toggle id={`oauth-${provider}-enabled`} label={`Enable ${labels[provider] ?? provider}`} checked={value.enabled} onChange={(enabled) => form.setValue('enabled', enabled, { shouldDirty: true, shouldValidate: true })} /></div><TextField form={form} name="clientId" label="Client ID" /><SecretEditor label={`${labels[provider] ?? provider} client secret`} configured={value.secretSet} secret={value.secret} error={errorAt(form.formState.errors, 'secret')} onChange={(secret) => form.setValue('secret', secret, { shouldDirty: true, shouldValidate: true })} />{special && <TextField form={form} name={`fields.${special}` as 'fields'} label={special} />}{value.enabled && <ReadOnlyField label="Callback URL" value={callback} copy={callback.startsWith('http')} />}<SectionSaveButton label={labels[provider] ?? provider} disabled={!form.formState.isDirty} /></form>
}
