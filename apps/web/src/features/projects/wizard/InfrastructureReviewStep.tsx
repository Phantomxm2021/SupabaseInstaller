import type { ReactNode } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { Alert } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { DatabaseRealtimeFields, GatewayNetworkFields, PoolerFields } from '../DatabaseNetworkStep'
import type { ProjectForm } from '../projectSchema'

export function InfrastructureReviewStep({ form }: { form: UseFormReturn<ProjectForm> }) {
  return <Card><CardHeader><CardTitle>Infrastructure settings</CardTitle><CardDescription>Review database, connection, and network settings before installation. Manager-controlled ports stay read-only.</CardDescription></CardHeader><CardContent className="space-y-3"><InfrastructureSection title="Database and Realtime settings"><DatabaseRealtimeFields form={form} /></InfrastructureSection><InfrastructureSection title="Connection pooler settings"><PoolerFields form={form} /></InfrastructureSection><InfrastructureSection title="Gateway and network settings"><GatewayNetworkFields form={form} /></InfrastructureSection><Alert>Manager validates unique ports and derives disabled service ports during installation.</Alert></CardContent></Card>
}

function InfrastructureSection({ title, children }: { title: string; children: ReactNode }) {
  return <Collapsible><CollapsibleTrigger className="flex w-full items-center justify-between rounded-lg border border-border px-4 py-3 text-left text-sm font-medium hover:bg-muted/50">{title}</CollapsibleTrigger><CollapsibleContent className="px-1 pt-4">{children}</CollapsibleContent></Collapsible>
}
