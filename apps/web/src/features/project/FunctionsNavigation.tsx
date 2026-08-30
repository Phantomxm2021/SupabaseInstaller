import { useLocation, useNavigate } from 'react-router-dom'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export function FunctionsNavigation({ projectId }: { projectId: string }) {
  const location = useLocation()
  const navigate = useNavigate()
  const value = location.pathname.endsWith('/secrets') ? 'secrets' : 'deployments'

  return <Tabs value={value} onValueChange={(next) => navigate(`/projects/${projectId}/functions${next === 'secrets' ? '/secrets' : ''}`)}>
    <TabsList aria-label="Functions navigation">
      <TabsTrigger value="deployments">Deployments</TabsTrigger>
      <TabsTrigger value="secrets">Secrets</TabsTrigger>
    </TabsList>
  </Tabs>
}
