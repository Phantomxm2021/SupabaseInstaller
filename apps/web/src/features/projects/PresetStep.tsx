import type { UseFormReturn } from 'react-hook-form'
import type { ProjectForm, PresetName } from './projectSchema'
import { applyPreset } from './projectSchema'

const names: Record<keyof ProjectForm['configuration']['services'], string> = { database:'PostgreSQL',gateway:'Envoy Gateway',auth:'Authentication',rest:'PostgREST',studio:'Supabase Studio',postgresMeta:'postgres-meta',realtime:'Realtime',storage:'Storage',imgproxy:'Image Transformation',functions:'Edge Functions',supavisor:'Supavisor',logs:'Logs & Analytics',vector:'Vector',directDb:'Direct PostgreSQL port' }
const presets: Array<[PresetName,string]> = [['LIGHTWEIGHT','Core database, gateway, Auth, REST and Studio.'],['STANDARD','Adds Realtime, Storage, Functions and Supavisor.'],['FULL','All official optional services, including Logs and image transformation.'],['CUSTOM','Choose every service individually.']]

export function PresetStep({ form }: { form: UseFormReturn<ProjectForm> }) {
  const preset = form.watch('preset')
  const services = form.watch('configuration.services')
  const setPreset = (next: PresetName) => { form.setValue('preset', next); form.setValue('configuration.services', applyPreset(next)) }
  const toggle = (name: keyof ProjectForm['configuration']['services']) => { const next = { ...services, [name]: !services[name] }; if (name==='storage'&&!next.storage) next.imgproxy=false; if (name==='logs') next.vector=next.logs; if (name==='vector') next.logs=next.vector; if (name==='studio') next.postgresMeta=next.studio; form.setValue('preset','CUSTOM'); form.setValue('configuration.services',next) }
  return <section className="wizard-section"><div className="section-heading"><span>02</span><div><h2>Preset & services</h2><p>Presets are editable; changes are saved as Custom.</p></div></div><div className="preset-grid">{presets.map(([key,description])=><button type="button" aria-label={key[0]+key.slice(1).toLowerCase()} key={key} className={`preset-card ${preset===key?'selected':''}`} onClick={()=>setPreset(key)}><strong>{key[0]+key.slice(1).toLowerCase()}</strong><p>{description}</p></button>)}</div><div className="form-grid services-grid">{(Object.keys(names) as Array<keyof typeof names>).map(name=><label key={name} className="service-toggle"><input type="checkbox" checked={services[name]} disabled={name==='database'||(name==='postgresMeta'&&services.studio)||(name==='vector'&&services.logs)||(name==='imgproxy'&&!services.storage)} onChange={()=>toggle(name)} /> <span>{names[name]}{name==='postgresMeta'&&services.studio?' (required by Studio)':name==='imgproxy'&&!services.storage?' (enable Storage first)':name==='vector'&&services.logs?' (managed with Logs)':''}</span></label>)}</div></section>
}
