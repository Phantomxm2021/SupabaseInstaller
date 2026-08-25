import { CheckCircle2, CircleOff } from 'lucide-react'
import type { ProjectForm } from './projectSchema'

const enabled = ['Database', 'Envoy Gateway', 'Authentication', 'PostgREST', 'Supabase Studio', 'postgres-meta']
const disabled = ['Realtime', 'Storage', 'Image Transformation', 'Edge Functions', 'Supavisor', 'Logs & Analytics']

export function ReviewStep({ project }: { project: ProjectForm }) {
  return (
    <section className="review-layout">
      <div className="review-summary panel">
        <div className="panel-heading"><h2>{project.name}</h2><p>Ready to install</p></div>
        <dl><div><dt>Domain</dt><dd>{project.domain}</dd></div><div><dt>Site URL</dt><dd>{project.siteUrl}</dd></div><div><dt>Preset</dt><dd><span className="badge healthy">Lightweight</span></dd></div><div><dt>Runtime</dt><dd>self-hosted/v0.8.0</dd></div><div><dt>Estimated containers</dt><dd>6</dd></div></dl>
      </div>
      <div className="service-review">
        <div><h3>Enabled services</h3>{enabled.map((service) => <span key={service}><CheckCircle2 size={15} />{service}</span>)}</div>
        <div className="disabled-services"><h3>Disabled by default</h3>{disabled.map((service) => <span key={service}><CircleOff size={15} />{service}</span>)}</div>
      </div>
    </section>
  )
}
