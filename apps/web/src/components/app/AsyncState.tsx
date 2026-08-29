import type { ReactNode } from "react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

type AsyncStateBaseProps = {
  className?: string
}

type LoadingStateProps = AsyncStateBaseProps & {
  variant: "loading"
}

type ErrorStateProps = AsyncStateBaseProps & {
  variant: "error"
  title?: ReactNode
  description?: ReactNode
  onRetry?: () => void
}

type EmptyStateProps = AsyncStateBaseProps & {
  variant: "empty"
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
}

export type AsyncStateProps =
  | LoadingStateProps
  | ErrorStateProps
  | EmptyStateProps

export function AsyncState(props: AsyncStateProps) {
  if (props.variant === "loading") {
    return (
      <div
        role="status"
        aria-live="polite"
        className={cn("h-24 w-full", props.className)}
      >
        <Skeleton aria-hidden="true" className="h-full w-full" />
        <span className="sr-only">Loading</span>
      </div>
    )
  }

  if (props.variant === "error") {
    return (
      <Alert variant="destructive" className={props.className}>
        <AlertTitle>{props.title ?? "Something went wrong"}</AlertTitle>
        {props.description ? <AlertDescription>{props.description}</AlertDescription> : null}
        {props.onRetry ? (
          <Button type="button" variant="outline" size="sm" className="mt-3" onClick={props.onRetry}>
            Retry
          </Button>
        ) : null}
      </Alert>
    )
  }

  return (
    <section className={cn("flex flex-col items-start gap-2 py-8", props.className)}>
      <h2 className="font-heading text-base font-medium text-foreground">{props.title}</h2>
      {props.description ? (
        <p className="text-sm text-muted-foreground">{props.description}</p>
      ) : null}
      {props.action ? <div className="mt-2">{props.action}</div> : null}
    </section>
  )
}
