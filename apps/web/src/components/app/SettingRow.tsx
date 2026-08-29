import { useId, type ReactNode } from "react"

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"

export type SettingRowProps = {
  label: ReactNode
  description?: ReactNode
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  className?: string
  disabled?: boolean
  error?: ReactNode
  children?: ReactNode
}

export function SettingRow({
  label,
  description,
  checked,
  onCheckedChange,
  className,
  disabled = false,
  error,
  children,
}: SettingRowProps) {
  const labelId = useId()
  const errorId = useId()

  return (
    <Card className={className}>
      <Collapsible>
        <CardHeader className="flex items-center gap-3 py-3">
          <CollapsibleTrigger className="min-w-0 flex-1 text-left">
            <CardTitle id={labelId}>{label}</CardTitle>
            {description ? <CardDescription>{description}</CardDescription> : null}
          </CollapsibleTrigger>
          <Switch
            aria-labelledby={labelId}
            aria-describedby={error ? errorId : undefined}
            aria-invalid={Boolean(error)}
            checked={checked}
            disabled={disabled}
            onClick={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
            onCheckedChange={onCheckedChange}
          />
        </CardHeader>
        {children || error ? (
          <CollapsibleContent>
            <CardContent className="border-t pt-4">
              {children}
              {error ? (
                <p id={errorId} role="alert" className="mt-3 text-sm text-destructive">
                  {error}
                </p>
              ) : null}
            </CardContent>
          </CollapsibleContent>
        ) : null}
      </Collapsible>
    </Card>
  )
}
