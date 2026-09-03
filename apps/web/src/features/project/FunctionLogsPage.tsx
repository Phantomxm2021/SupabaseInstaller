import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Pause, Play, RefreshCw, Search } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import type { FunctionLogHealth, FunctionLogLevel, FunctionLogPage, FunctionLogRecord } from '@/api/types'
import { PageHeader } from '@/components/app/PageHeader'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

type LevelFilter = '' | FunctionLogLevel

export function FunctionLogsPage() {
  const { projectId = '', functionName = '' } = useParams()
  const [level, setLevel] = useState<LevelFilter>('')
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [paused, setPaused] = useState(false)

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search), 300)
    return () => window.clearTimeout(timer)
  }, [search])

  const projectPath = encodeURIComponent(projectId)
  const functionPath = encodeURIComponent(functionName)
  return <main className="page function-logs-page" data-density="dashboard">
    <Link className="function-logs-back" to={`/projects/${projectPath}/functions`}><ArrowLeft /> Back to functions</Link>
    <PageHeader eyebrow="Edge Functions / Logs" title={`${functionName} logs`} description="Inspect retained runtime events for this function." actions={<>
      <Button variant="outline" onClick={() => setPaused((value) => !value)}>{paused ? <Play /> : <Pause />}{paused ? 'Resume' : 'Pause'} auto refresh</Button>
    </>} />
    <div className="function-logs-toolbar">
      <label><span>Level</span><select aria-label="Log level" value={level} onChange={(event) => setLevel(event.target.value as LevelFilter)}><option value="">All levels</option><option value="debug">Debug</option><option value="info">Info</option><option value="warn">Warn</option><option value="error">Error</option></select></label>
      <label className="function-logs-search"><span className="sr-only">Search logs</span><Search /><Input type="search" aria-label="Search logs" placeholder="Search messages" value={search} onChange={(event) => setSearch(limitUTF8(event.target.value, 256))} /></label>
    </div>
    <LogResults key={`${level}\u0000${debouncedSearch}`} projectId={projectId} functionName={functionName} level={level} search={debouncedSearch} paused={paused} />
  </main>
}

function LogResults({ projectId, functionName, level, search, paused }: { projectId: string; functionName: string; level: LevelFilter; search: string; paused: boolean }) {
  const [records, setRecords] = useState<FunctionLogRecord[]>([])
  const [health, setHealth] = useState<FunctionLogHealth | null>(null)
  const [olderCursor, setOlderCursor] = useState('')
  const [olderPending, setOlderPending] = useState(false)
  const [olderError, setOlderError] = useState('')
  const newerCursor = useRef('')
  const mounted = useRef(true)
  const olderRequest = useRef<AbortController | null>(null)
  const base = `/api/projects/${encodeURIComponent(projectId)}/functions/${encodeURIComponent(functionName)}/logs`
  const query = useQuery({
    queryKey: ['function-logs', projectId, functionName, level, search],
    queryFn: ({ signal }) => apiFetch<FunctionLogPage>(makeURL(base, { limit: '200', level, search, after: newerCursor.current }), { signal }),
    enabled: Boolean(projectId && functionName),
    retry: false,
    refetchInterval: paused ? false : 5_000,
  })

  useEffect(() => () => {
    mounted.current = false
    olderRequest.current?.abort()
  }, [])

  useEffect(() => {
    const page = query.data
    if (!page) return
    setRecords((current) => mergeLogs(current, page.logs))
    setHealth(page.health)
    if (page.newerCursor) newerCursor.current = page.newerCursor
    if (page.olderCursor) setOlderCursor((current) => current || page.olderCursor)
  }, [query.data, query.dataUpdatedAt])

  async function loadOlder() {
    if (!olderCursor || olderPending) return
    setOlderPending(true); setOlderError('')
    const controller = new AbortController()
    olderRequest.current = controller
    try {
      const page = await apiFetch<FunctionLogPage>(makeURL(base, { limit: '200', level, search, before: olderCursor }), { signal: controller.signal })
      if (!mounted.current) return
      setRecords((current) => mergeLogs(current, page.logs))
      setHealth(page.health)
      setOlderCursor(page.olderCursor)
    } catch (error) {
      if (!mounted.current || controller.signal.aborted) return
      setOlderError(error instanceof Error ? error.message : 'Could not load older logs')
    } finally {
      if (mounted.current) setOlderPending(false)
      if (olderRequest.current === controller) olderRequest.current = null
    }
  }

  if (query.isPending && records.length === 0) return <Card><CardContent className="function-logs-status">Loading function logs…</CardContent></Card>
  if (query.isError && records.length === 0) return <Alert variant="destructive"><span>Could not load function logs. {query.error.message}</span><Button variant="outline" size="sm" onClick={() => void query.refetch()}>Retry</Button></Alert>

  return <>
    {health && <HealthNotice health={health} />}
    {query.isError && records.length > 0 && <Alert variant="destructive">Could not refresh logs. Previously loaded records are still shown.</Alert>}
    {olderError && <Alert variant="destructive">{olderError}</Alert>}
    <div className="function-logs-actions"><span>{records.length} retained {records.length === 1 ? 'event' : 'events'}</span><Button variant="outline" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}><RefreshCw className={query.isFetching ? 'animate-spin' : ''} /> Refresh</Button></div>
    <Card className="function-logs-card"><CardContent className="function-logs-table-wrap">
      {records.length === 0 && health?.status === 'healthy' ? <div className="function-logs-empty"><p>No retained logs.</p><span>Runtime events will appear here as they are collected.</span></div> : <Table><TableHeader><TableRow><TableHead>Timestamp</TableHead><TableHead>Level</TableHead><TableHead>Event type</TableHead><TableHead>Message</TableHead></TableRow></TableHeader><TableBody>{records.map((record) => <TableRow key={record.id}><TableCell className="function-log-timestamp">{formatTimestamp(record.timestamp)}</TableCell><TableCell><Badge variant={record.level === 'error' ? 'destructive' : 'outline'}>{record.level}</Badge></TableCell><TableCell>{record.eventType}</TableCell><TableCell><pre className="function-log-message">{record.message}{record.truncated ? ' …' : ''}</pre></TableCell></TableRow>)}</TableBody></Table>}
    </CardContent></Card>
    {olderCursor && <div className="function-logs-older"><Button variant="outline" disabled={olderPending} onClick={() => void loadOlder()}>{olderPending ? 'Loading…' : 'Load older'}</Button></div>}
  </>
}

function HealthNotice({ health }: { health: FunctionLogHealth }) {
  const messages: Record<FunctionLogHealth['status'], string> = {
    healthy: 'Log collection healthy',
    dropped: `${health.dropped} log events were dropped; retained records may be incomplete.`,
    offline: 'Log collector is offline or stale.',
    incompatible: 'The runtime logging adapter is incompatible.',
    disabled: 'Functions are disabled. Retained records remain available.',
    not_installed: 'Log collection is not installed. Reconcile this project to install it.',
    storage_error: 'Log storage is unavailable. Previously retained records may still be shown.',
  }
  const warning = health.status !== 'healthy'
  return <Alert variant={warning ? 'destructive' : 'default'}><span>{messages[health.status]}{health.rejected > 0 ? ` ${health.rejected} events were rejected.` : ''}{health.detail ? ` ${health.detail}` : ''}</span></Alert>
}

function makeURL(base: string, values: Record<string, string>) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) if (value) params.set(key, value)
  return `${base}?${params.toString()}`
}

function mergeLogs(current: FunctionLogRecord[], incoming: FunctionLogRecord[]) {
  const unique = new Map(current.map((record) => [record.id, record]))
  for (const record of incoming) unique.set(record.id, record)
  return [...unique.values()].sort((a, b) => b.timestamp.localeCompare(a.timestamp) || b.id.localeCompare(a.id))
}

function limitUTF8(value: string, maxBytes: number) {
  let result = ''
  let bytes = 0
  for (const character of value) {
    const size = new TextEncoder().encode(character).length
    if (bytes + size <= maxBytes) { result += character; bytes += size }
  }
  return result
}

function formatTimestamp(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleString()
}
