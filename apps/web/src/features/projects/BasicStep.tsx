import type { UseFormReturn } from 'react-hook-form'
import type { ProjectForm } from './projectSchema'

export function BasicStep({ form }: { form: UseFormReturn<ProjectForm> }) {
  return (
    <section className="wizard-section">
      <div className="section-heading"><span>01</span><div><h2>Project details</h2><p>The three fields required for a Lightweight install.</p></div></div>
      <div className="form-grid">
        <div><label>Project name<input autoFocus placeholder="Bee" {...form.register('name')} /></label><ErrorText value={form.formState.errors.name?.message} /></div>
        <div><label>Project slug<input placeholder="bee" {...form.register('slug')} /></label><ErrorText value={form.formState.errors.slug?.message} /></div>
        <div><label>Domain<input placeholder="bee.example.com" {...form.register('configuration.general.domain')} /></label><ErrorText value={form.formState.errors.configuration?.general?.domain?.message} /></div>
        <div><label>Site URL<input placeholder="https://example.com" {...form.register('configuration.general.siteUrl')} /></label><ErrorText value={form.formState.errors.configuration?.general?.siteUrl?.message} /></div>
        <div><label>Pinned Supabase version<select {...form.register('configuration.general.supabaseVersion')}><option value="self-hosted/v0.8.0">self-hosted/v0.8.0</option></select></label></div>
      </div>
    </section>
  )
}

function ErrorText({ value }: { value?: string }) { return value ? <span className="field-error">{value}</span> : null }
