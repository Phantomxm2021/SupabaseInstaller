import { NavLink, useParams } from 'react-router-dom'

export const authenticationGroups = [
  { label: 'Manage', items: [['users', 'Users'], ['oauth-apps', 'OAuth Apps']] },
  { label: 'Notifications', items: [['emails', 'Emails']] },
  { label: 'Configuration', items: [['sign-in-providers', 'Sign In / Providers'], ['sessions', 'Sessions'], ['rate-limits', 'Rate Limits'], ['multi-factor', 'Multi-Factor'], ['url-configuration', 'URL Configuration'], ['attack-protection', 'Attack Protection'], ['auth-hooks', 'Auth Hooks'], ['audit-logs', 'Audit Logs'], ['performance', 'Performance']] },
] as const

export function AuthenticationNavigation() {
  const { projectId = '' } = useParams()

  return <nav aria-label="Authentication navigation" className="authentication-navigation">
    {authenticationGroups.map((group) => <section className="authentication-navigation-group" key={group.label} aria-labelledby={`authentication-${group.label.toLowerCase()}`}>
      <h2 id={`authentication-${group.label.toLowerCase()}`} className="authentication-navigation-label">{group.label.toUpperCase()}</h2>
      <ul className="authentication-navigation-list">
        {group.items.map(([path, label]) => <li key={path}>
          <NavLink to={`/projects/${projectId}/authentication/${path}`} className="authentication-navigation-link">{label}</NavLink>
        </li>)}
      </ul>
    </section>)}
  </nav>
}
