import { Check, Minus } from 'lucide-react'
import type { Services } from '../../api/types'

const serviceNames: Array<[keyof Services, string]> = [
  ['database', 'Postgres database'], ['gateway', 'API gateway'], ['auth', 'Authentication'], ['rest', 'REST API'],
  ['studio', 'Studio'], ['postgresMeta', 'Postgres Meta'], ['realtime', 'Realtime'], ['storage', 'Storage'],
  ['imgproxy', 'Image proxy'], ['functions', 'Edge Functions'], ['supavisor', 'Supavisor'], ['logs', 'Logs'], ['vector', 'Vector'], ['directDb', 'Direct database access'],
]

export function ServiceTable({ services }: { services: Services }) {
  return (
    <div className="service-table">
      {serviceNames.map(([key, label]) => <div className="service-row" key={key}><span>{label}</span><span className={services[key] ? 'service-enabled' : 'service-disabled'}>{services[key] ? <><Check size={14} /> Enabled</> : <><Minus size={14} /> Disabled</>}</span></div>)}
    </div>
  )
}
