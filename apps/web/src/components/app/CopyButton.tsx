import { Check, Copy } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export function CopyButton({ value, label, className }: { value: string; label: string; className?: string }) {
  const [copied, setCopied] = useState(false)
  const resetTimer = useRef<number | undefined>(undefined)
  const actionLabel = `${copied ? 'Copied' : 'Copy'} ${label}`

  useEffect(() => () => window.clearTimeout(resetTimer.current), [])

  const copy = async () => {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard is unavailable')
      await navigator.clipboard.writeText(value)
      setCopied(true)
      toast.success(`${label} copied`)
      window.clearTimeout(resetTimer.current)
      resetTimer.current = window.setTimeout(() => setCopied(false), 2_000)
    } catch {
      toast.error(`Unable to copy ${label}`)
    }
  }

  return <Button type="button" variant="outline" size="icon" aria-label={actionLabel} title={actionLabel} className={cn(copied && 'border-emerald-500/50 bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15 dark:text-emerald-400', className)} onClick={() => void copy()}>{copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}</Button>
}
