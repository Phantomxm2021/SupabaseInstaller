import type { UseFormReturn } from 'react-hook-form'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldLabel, FieldGroup } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { defaultConfiguration, type PresetName, type ProjectForm } from '../projectSchema'
import { serviceControlState, serviceGroups, serviceLabels, servicePresets, setServiceEnabled, type ServiceName } from '../PresetStep'

export function ServiceConfiguration({ form }: { form: UseFormReturn<ProjectForm> }) {
  const preset = form.watch('preset')
  const services = form.watch('configuration.services')
  const httpsMode = form.watch('configuration.network.httpsMode')
  const enabledCount = Object.values(services).filter(Boolean).length
  const serviceError = (name: ServiceName) => (form.formState.errors.configuration?.services as Record<string, { message?: string }> | undefined)?.[name]?.message
  const setPreset = (next: PresetName) => {
    const current = form.getValues('configuration')
    const reset = defaultConfiguration(next)
    // Presets are aggregate defaults; preserve only identity fields entered on step 1.
    reset.general = current.general
    form.setValue('preset', next, { shouldDirty: true })
    form.setValue('configuration', reset as ProjectForm['configuration'], { shouldDirty: true, shouldValidate: true })
  }

  return <Card className="service-configuration"><CardHeader><CardTitle>Preset & services</CardTitle><CardDescription>Start with a preset, then tailor individual services. Dependencies are applied automatically.</CardDescription></CardHeader><CardContent><div className="service-configuration-layout">
    <nav aria-label="Service presets" className="service-preset-nav">
      {servicePresets.map((item) => <Button key={item.name} type="button" variant={preset === item.name ? 'default' : 'ghost'} aria-label={item.label} aria-current={preset === item.name ? 'true' : undefined} className="service-preset-option" onClick={() => setPreset(item.name)}><span className="font-semibold">{item.label}</span><span className="service-preset-description">{item.description}</span></Button>)}
    </nav>
    <div className="service-configuration-content">
      <div className="service-configuration-summary"><div><h2 className="text-base font-semibold">Services</h2><p className="text-sm text-muted-foreground" aria-live="polite">{enabledCount} of {Object.keys(serviceLabels).length} services enabled</p></div>{preset === 'CUSTOM' && <p className="text-sm text-muted-foreground">Custom keeps your service choices when you continue editing.</p>}</div>
      {serviceGroups.map((group) => <section key={group.label} aria-labelledby={`service-group-${group.label}`} className="service-group"><h2 id={`service-group-${group.label}`} className="service-group-heading">{group.label}</h2><FieldGroup className="service-group-controls">
        {group.names.map((name) => {
          const { forced, help } = serviceControlState(services, httpsMode, name)
          return <Field key={name} className="service-control"><div className="flex items-center justify-between gap-4"><div className="min-w-0"><FieldLabel htmlFor={`service-${name}`}>{serviceLabels[name]}{forced ? ' (required)' : ''}</FieldLabel>{help && <FieldDescription>{help}</FieldDescription>}</div><Switch id={`service-${name}`} checked={services[name]} disabled={forced} onCheckedChange={(checked) => setServiceEnabled(form, name, checked)} /></div><FieldError>{serviceError(name)}</FieldError></Field>
        })}
      </FieldGroup></section>)}
    </div>
  </div></CardContent></Card>
}
