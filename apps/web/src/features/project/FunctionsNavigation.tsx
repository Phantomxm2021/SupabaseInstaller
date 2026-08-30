import { PanelLeft } from 'lucide-react'
import { useEffect, useState } from 'react'
import { NavLink, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'

const compactBreakpoint = 768

function useCompactFunctionsNavigation() {
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

export function FunctionsNavigation() {
  const { projectId = '' } = useParams()
  const isCompact = useCompactFunctionsNavigation()
  const [open, setOpen] = useState(false)
  const navigation = <FunctionsNavigationContent projectId={projectId} onNavigate={() => setOpen(false)} />

  if (isCompact) return <>
    <Button variant="outline" className="functions-navigation-trigger" aria-haspopup="dialog" aria-expanded={open} aria-controls="functions-navigation-panel" onClick={() => setOpen((value) => !value)}><PanelLeft /> {open ? 'Close Functions navigation' : 'Open Functions navigation'}</Button>
    <Sheet open={open} onOpenChange={setOpen}><SheetContent id="functions-navigation-panel" side="left" className="functions-navigation-sheet"><SheetHeader><SheetTitle>Functions navigation</SheetTitle></SheetHeader>{navigation}</SheetContent></Sheet>
  </>

  return navigation
}

function FunctionsNavigationContent({ projectId, onNavigate }: { projectId: string; onNavigate?: () => void }) {
  return <nav aria-label="Functions navigation" className="functions-navigation">
    <header className="functions-navigation-title"><h1>Edge Functions</h1></header>
    <section className="functions-navigation-group" aria-labelledby="functions-workspace">
      <h2 id="functions-workspace" className="functions-navigation-label">WORKSPACE</h2>
      <ul className="functions-navigation-list">
        <li><NavLink end to={`/projects/${projectId}/functions`} className="functions-navigation-link" onClick={onNavigate}><span>Functions</span></NavLink></li>
        <li><NavLink to={`/projects/${projectId}/functions/secrets`} className="functions-navigation-link" onClick={onNavigate}><span>Secrets</span></NavLink></li>
      </ul>
    </section>
  </nav>
}
