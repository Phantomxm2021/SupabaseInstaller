"use client"

import { RadioGroup as RadioGroupPrimitive } from "@base-ui/react/radio-group"
import { Radio } from "@base-ui/react/radio"

import { cn } from "@/lib/utils"

function RadioGroup({ className, ...props }: RadioGroupPrimitive.Props<string>) {
  return <RadioGroupPrimitive data-slot="radio-group" className={cn("grid gap-2", className)} {...props} />
}

function RadioGroupItem({ className, ...props }: Radio.Root.Props<string>) {
  return (
    <Radio.Root
      data-slot="radio-group-item"
      className={cn(
        "relative size-4 shrink-0 rounded-full border border-input outline-none after:absolute after:-inset-3 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 data-checked:border-primary data-checked:bg-primary disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    >
      <Radio.Indicator data-slot="radio-group-indicator" className="absolute inset-1 rounded-full bg-primary-foreground" />
    </Radio.Root>
  )
}

export { RadioGroup, RadioGroupItem }
