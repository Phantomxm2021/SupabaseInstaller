import { QueryClient, useQuery } from '@tanstack/react-query'
import { Navigate, Outlet, createBrowserRouter, useParams } from 'react-router-dom'
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
            { path: 'services', element: <LegacyConfigurationRedirect section="services" /> },
            { path: 'authentication', element: <LegacyConfigurationRedirect section="auth" /> },
            { path: 'database', element: <LegacyConfigurationRedirect section="database" /> },
            { path: 'storage', element: <LegacyConfigurationRedirect section="storage" /> },
            { path: 'realtime', element: <LegacyConfigurationRedirect section="realtime" /> },
            { path: 'functions', element: <LegacyConfigurationRedirect section="functions" /> },
            { path: 'pooler', element: <LegacyConfigurationRedirect section="pooler" /> },
            { path: 'logs', element: <LegacyConfigurationRedirect section="services" /> },
            { path: 'network', element: <LegacyConfigurationRedirect section="network" /> },
            { path: 'secrets', element: <LegacyConfigurationRedirect section="secrets" /> },
            { path: 'settings', element: <LegacyConfigurationRedirect section="general" /> },
            { path: '*', element: <ComingSoonPage /> },
          ],
        },
        { path: '*', element: <Outlet /> },
      ],
    },
  ])
}

function LegacyConfigurationRedirect({ section }: { section: string }) {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}/configuration?section=${section}`} replace />
}
