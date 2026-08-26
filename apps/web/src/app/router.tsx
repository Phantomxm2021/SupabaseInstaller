import { QueryClient, useQuery } from '@tanstack/react-query'
import { Navigate, Outlet, createBrowserRouter } from 'react-router-dom'
import { apiFetch } from '../api/client'
import { sessionQueryOptions } from '../api/session'
import { LoginPage } from '../features/auth/LoginPage'
import { SetupPage } from '../features/auth/SetupPage'
import { ProjectsPage } from '../features/projects/ProjectsPage'
import { NewProjectPage } from '../features/projects/NewProjectPage'
import { OverviewPage } from '../features/project/OverviewPage'
import { ConfigurationPage } from '../features/project/ConfigurationPage'
import { ComingSoonPage, ProjectLayout } from '../features/project/ProjectLayout'
import { AppShell } from './AppShell'
import { ManagerSettingsPage } from '../features/settings/ManagerSettingsPage'
import { AuthenticationWorkspace, EmailsRoute, EmailTemplateEditorRoute, MultiFactorRoute, RateLimitsRoute, SignInProvidersRoute, URLConfigurationRoute } from '../features/authentication/AuthenticationWorkspace'
import { AuthenticationUnavailablePage } from '../features/authentication/AuthenticationUnavailablePage'
import { unsupportedAuthenticationRoutes } from '../features/authentication/navigation'

function EntryGate() {
  const setup = useQuery({ queryKey: ['setup-status'], queryFn: () => apiFetch<{ required: boolean }>('/api/setup/status') })
  if (setup.isLoading) return <div className="splash">Starting Supabase Manager…</div>
  if (setup.data?.required) return <Navigate to="/setup" replace />
  return <Navigate to="/projects" replace />
}

export function AuthenticatedShell() {
  const session = useQuery({ ...sessionQueryOptions(), retry: false })
  if (session.isLoading) return <div className="splash">Loading workspace…</div>
  if (session.isError) return <Navigate to="/login" replace />
  return <AppShell />
}

export function createAppRouter(_queryClient: QueryClient) {
  return createBrowserRouter([
    { path: '/', element: <EntryGate /> },
    { path: '/setup', element: <SetupPage /> },
    { path: '/login', element: <LoginPage /> },
    {
      element: <AuthenticatedShell />,
      children: [
        { path: '/projects', element: <ProjectsPage /> },
        { path: '/settings', element: <ManagerSettingsPage /> },
        { path: '/projects/new', element: <NewProjectPage /> },
        {
          path: '/projects/:projectId',
          element: <ProjectLayout />,
          children: [
            { index: true, element: <Navigate to="overview" replace /> },
            { path: 'overview', element: <OverviewPage /> },
            { path: 'configuration', element: <ConfigurationPage /> },
            {
              path: 'authentication',
              element: <AuthenticationWorkspace />,
              children: [
                { index: true, element: <Navigate to="sign-in-providers" replace /> },
                { path: 'sign-in-providers', element: <SignInProvidersRoute /> },
                { path: 'emails', element: <EmailsRoute /> },
                { path: 'emails/:templateKey', element: <EmailTemplateEditorRoute /> },
                { path: 'rate-limits', element: <RateLimitsRoute /> },
                { path: 'multi-factor', element: <MultiFactorRoute /> },
                { path: 'url-configuration', element: <URLConfigurationRoute /> },
                ...unsupportedAuthenticationRoutes.map(([path, label]) => ({ path, element: <AuthenticationUnavailablePage title={label} /> })),
              ],
            },
            { path: 'logs', element: <ComingSoonPage /> },
            { path: 'backups', element: <ComingSoonPage /> },
            { path: '*', element: <NotFoundPage /> },
          ],
        },
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ])
}

export function NotFoundPage() {
  return <main className="page"><h1>Page not found</h1><p className="muted">The requested Manager page does not exist.</p></main>
}
