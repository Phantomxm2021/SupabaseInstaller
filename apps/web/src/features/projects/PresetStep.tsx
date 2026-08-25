import { Check, Layers3, LockKeyhole } from 'lucide-react'

export function PresetStep() {
  return (
    <section className="wizard-section">
      <div className="section-heading"><span>02</span><div><h2>Configuration</h2><p>Optional services stay off to minimize resource usage.</p></div></div>
      <div className="preset-grid">
        <div className="preset-card selected"><div className="preset-title"><Layers3 size={18} /><strong>Lightweight</strong><Check size={17} /></div><p>Database, Envoy, Auth, REST, Studio and postgres-meta.</p><span className="recommended">Recommended</span></div>
        <div className="preset-card disabled"><div className="preset-title"><LockKeyhole size={18} /><strong>Standard</strong></div><p>Add Realtime, Storage, Functions and Pooler.</p><span>Available in Service Manager</span></div>
        <div className="preset-card disabled"><div className="preset-title"><LockKeyhole size={18} /><strong>Full</strong></div><p>Every official optional service.</p><span>Available in Service Manager</span></div>
      </div>
    </section>
  )
}
