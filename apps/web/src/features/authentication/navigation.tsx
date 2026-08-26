import { ExternalLink, PanelLeft } from 'lucide-react'
import { useEffect, useState } from 'react'
import { NavLink, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'

type AuthenticationItem = readonly [path: string, label: string, options?: { beta?: boolean; external?: boolean }]
type AuthenticationGroup = { label: string; items: readonly AuthenticationItem[] }

export const authenticationGroups: readonly AuthenticationGroup[] = [
  { label: 'Manage', items: [['users', 'Users'], ['oauth-apps', 'OAuth Apps']] },
  { label: 'Notifications', items: [['emails', 'Emails']] },
  { label: 'Configuration', items: [['policies', 'Policies', { external: true }], ['sign-in-providers', 'Sign In / Providers'], ['passkeys', 'Passkeys', { beta: true }], ['oauth-server', 'OAuth Server', { beta: true }], ['sessions', 'Sessions'], ['rate-limits', 'Rate Limits'], ['multi-factor', 'Multi-Factor'], ['url-configuration', 'URL Configuration'], ['attack-protection', 'Attack Protection'], ['auth-hooks', 'Auth Hooks', { beta: true }], ['audit-logs', 'Audit Logs'], ['performance', 'Performance']] },
] as const

export const unsupportedAuthenticationRoutes = authenticationGroups.flatMap((group) => group.items).filter(([path]) => !['emails', 'sign-in-providers', 'rate-limits', 'multi-factor', 'url-configuration'].includes(path))

const compactBreakpoint = 900

function useCompactAuthenticationNavigation() {
  const [isCompact, setIsCompact] = useState(() => typeof window !== 'undefined' && window.innerWidth <= compactBreakpoint)

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const query = window.matchMedia(`(max-width: ${compactBreakpoint}px)`)
    const update = () => setIsCompact(query.matches)
    update()
    query.addEventListener('change', update)
    return () => query.removeEventListener('change', update)
  }, [])

  return isCompact
}

export function AuthenticationNavigation() {
  const { projectId = '' } = useParams()
  const isCompact = useCompactAuthenticationNavigation()
  const [open, setOpen] = useState(false)

  const navigation = <AuthenticationNavigationContent projectId={projectId} onNavigate={() => setOpen(false)} />

  if (isCompact) return <>
    <Button variant="outline" className="authentication-navigation-trigger" aria-haspopup="dialog" aria-expanded={open} aria-controls="authentication-navigation-panel" onClick={() => setOpen((value) => !value)}>
      <PanelLeft /> {open ? 'Close authentication navigation' : 'Open authentication navigation'}
    </Button>
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent id="authentication-navigation-panel" side="left" className="authentication-navigation-sheet">
        <SheetHeader><SheetTitle>Authentication navigation</SheetTitle></SheetHeader>
        {navigation}
      </SheetContent>
    </Sheet>
  </>

  return navigation
}

function AuthenticationNavigationContent({ projectId, onNavigate }: { projectId: string; onNavigate?: () => void }) {
  return <nav aria-label="Authentication navigation" className="authentication-navigation">
    <header className="authentication-navigation-title"><h1>Authentication</h1></header>
    {authenticationGroups.map((group) => <section className="authentication-navigation-group" key={group.label} aria-labelledby={`authentication-${group.label.toLowerCase()}`}>
      <h2 id={`authentication-${group.label.toLowerCase()}`} className="authentication-navigation-label">{group.label.toUpperCase()}</h2>
      <ul className="authentication-navigation-list">
        {group.items.map(([path, label, options]) => <li key={path}>
          <NavLink to={`/projects/${projectId}/authentication/${path}`} className="authentication-navigation-link" data-external={options?.external || undefined} onClick={onNavigate}><span>{label}</span>{options?.beta && <span className="authentication-navigation-beta">BETA</span>}{options?.external && <ExternalLink aria-hidden="true" />}</NavLink>
        </li>)}
      </ul>
    </section>)}
  </nav>
}
