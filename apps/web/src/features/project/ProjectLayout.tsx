import { Outlet } from 'react-router-dom'
import { Card, CardContent } from '@/components/ui/card'

export function ProjectLayout() {
  return <Outlet />
}

export function ComingSoonPage() {
  return <main className="page"><Card><CardContent className="empty-state"><h3>This server module is coming next</h3><p>The runtime is already managed from Overview.</p></CardContent></Card></main>
}
