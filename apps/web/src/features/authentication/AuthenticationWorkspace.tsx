import { Outlet } from 'react-router-dom'

export function AuthenticationWorkspace() {
  return <section className="authentication-workspace">
    <main className="authentication-content"><Outlet /></main>
  </section>
}

export function SignInProvidersRoute() {
  return <main className="page"><h1>Sign In / Providers</h1></main>
}

export function EmailsRoute() {
  return <main className="page"><h1>Emails</h1></main>
}

export function URLConfigurationRoute() {
  return <main className="page"><h1>URL Configuration</h1></main>
}
