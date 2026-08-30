import { Outlet } from 'react-router-dom'
import { FunctionsNavigation } from './FunctionsNavigation'

export function FunctionsWorkspace() {
  return <section className="functions-workspace"><FunctionsNavigation /><div className="functions-content"><Outlet /></div></section>
}
