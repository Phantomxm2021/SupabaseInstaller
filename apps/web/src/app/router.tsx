import { QueryClient, useQuery } from '@tanstack/react-query'
import { Navigate, Outlet, createBrowserRouter } from 'react-router-dom'
import { apiFetch, setCSRFToken } from '../api/client'
import { LoginPage } from '../features/auth/LoginPage'
import { SetupPage } from '../features/auth/SetupPage'
import { ProjectsPage } from '../features/projects/ProjectsPage'
import { NewProjectPage } from '../features/projects/NewProjectPage'
import { OverviewPage } from '../features/project/OverviewPage'
import { ComingSoonPage, ProjectLayout } from '../features/project/ProjectLayout'
import { AppShell } from './AppShell'

function EntryGate() {
  const setup = useQuery({ queryKey: ['setup-status'], queryFn: () => apiFetch<{ required: boolean }>('/api/setup/status') })
  if (setup.isLoading) return <div className="splash">Starting Supabase Manager…</div>
  if (setup.data?.required) return <Navigate to="/setup" replace />
  return <Navigate to="/projects" replace />
}

function AuthenticatedShell() {
  const session = useQuery({
    queryKey: ['session'],
    queryFn: async () => {
      const current = await apiFetch<{ csrfToken: string }>('/api/session')
      setCSRFToken(current.csrfToken)
      return current
    },
    retry: false,
  })
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
        { path: '/projects/new', element: <NewProjectPage /> },
        {
          path: '/projects/:projectId',
          element: <ProjectLayout />,
          children: [
            { index: true, element: <Navigate to="overview" replace /> },
            { path: 'overview', element: <OverviewPage /> },
            { path: '*', element: <ComingSoonPage /> },
          ],
        },
        { path: '*', element: <Outlet /> },
      ],
    },
  ])
}
