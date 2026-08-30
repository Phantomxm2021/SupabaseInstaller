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
    <header
      data-density="dashboard"
      className={cn("dashboard-page-header flex w-full items-start justify-between gap-4 max-[560px]:flex-col max-[560px]:items-start", className)}
    >
      <div className="min-w-0 space-y-1">
        {eyebrow ? (
          <p className="font-mono text-[12px] leading-4 font-semibold tracking-wide text-primary uppercase">
            {eyebrow}
          </p>
        ) : null}
        <h1 className="font-heading text-[22px] leading-[29px] font-semibold tracking-[-0.025em] text-foreground">
          {title}
        </h1>
        {description ? (
          <p className="text-[13px] leading-[19px] text-muted-foreground">{description}</p>
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
