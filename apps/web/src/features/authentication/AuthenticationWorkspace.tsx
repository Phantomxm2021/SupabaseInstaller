import { Outlet } from 'react-router-dom'
import { AuthenticationNavigation } from './navigation'

export function AuthenticationWorkspace() {
  return <section className="authentication-workspace">
    <AuthenticationNavigation />
    <div className="authentication-content"><Outlet /></div>
  </section>
}

export function SignInProvidersRoute() {
  return <section className="page"><h1>Sign In / Providers</h1></section>
}

export function EmailsRoute() {
  return <section className="page"><h1>Emails</h1></section>
}

export function URLConfigurationRoute() {
  return <section className="page"><h1>URL Configuration</h1></section>
}
