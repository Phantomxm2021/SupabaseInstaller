import type { ReactNode } from "react"

import { cn } from "@/lib/utils"

export type PageHeaderProps = {
  eyebrow?: ReactNode
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <header className={cn("flex items-start justify-between gap-4", className)}>
      <div className="min-w-0 space-y-1">
        {eyebrow ? (
          <p className="text-xs font-medium tracking-wide text-primary uppercase">
            {eyebrow}
          </p>
        ) : null}
        <h1 className="font-heading text-2xl font-semibold tracking-tight text-foreground">
          {title}
        </h1>
        {description ? (
          <p className="text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {actions ? (
        <div
          data-slot="page-header-actions"
          className="flex shrink-0 items-center gap-2"
        >
          {actions}
        </div>
      ) : null}
    </header>
  )
}
