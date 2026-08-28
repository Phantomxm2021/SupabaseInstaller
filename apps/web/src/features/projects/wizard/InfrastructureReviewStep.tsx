import { useEffect, useState, type ReactNode } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { Alert } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { DatabaseRealtimeFields, GatewayNetworkFields, PoolerFields } from '../DatabaseNetworkStep'
import type { ProjectForm } from '../projectSchema'

export type InfrastructureSectionId = 'database-realtime' | 'pooler' | 'gateway-network'

export function InfrastructureReviewStep({ form, openedSection }: { form: UseFormReturn<ProjectForm>; openedSection?: InfrastructureSectionId }) {
  return <Card><CardHeader><CardTitle>Infrastructure settings</CardTitle><CardDescription>Review database, connection, and network settings before installation. Manager-controlled ports stay read-only.</CardDescription></CardHeader><CardContent className="space-y-3"><InfrastructureSection id="database-realtime" title="Database and Realtime settings" openedSection={openedSection}><DatabaseRealtimeFields form={form} /></InfrastructureSection><InfrastructureSection id="pooler" title="Connection pooler settings" openedSection={openedSection}><PoolerFields form={form} /></InfrastructureSection><InfrastructureSection id="gateway-network" title="Gateway and network settings" openedSection={openedSection}><GatewayNetworkFields form={form} /></InfrastructureSection><Alert>Manager validates unique ports and derives disabled service ports during installation.</Alert></CardContent></Card>
}

function InfrastructureSection({ id, title, children, openedSection }: { id: InfrastructureSectionId; title: string; children: ReactNode; openedSection?: InfrastructureSectionId }) {
  const [open, setOpen] = useState(false)
  useEffect(() => { if (openedSection === id) setOpen(true) }, [id, openedSection])
  return <Collapsible open={open || openedSection === id} onOpenChange={setOpen} data-infrastructure-section={id}><CollapsibleTrigger className="flex w-full items-center justify-between rounded-lg border border-border px-4 py-3 text-left text-sm font-medium hover:bg-muted/50">{title}</CollapsibleTrigger><CollapsibleContent className="px-1 pt-4">{children}</CollapsibleContent></Collapsible>
}
