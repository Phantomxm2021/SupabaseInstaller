import * as React from "react"
import { Input as InputPrimitive } from "@base-ui/react/input"

import { cn } from "@/lib/utils"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <InputPrimitive
      type={type}
      data-slot="input"
      data-density="dashboard"
      className={cn(
        "h-[34px] w-full min-w-0 rounded-[6px] border border-input bg-input/25 px-3 py-2 font-mono text-[13px] leading-[18px] font-[450] transition-[background-color,border-color,box-shadow] outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:font-sans file:text-[12px] file:font-[450] file:text-foreground placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/35 disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-input/50 disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-2 aria-invalid:ring-destructive/30 dark:aria-invalid:border-destructive/50",
        className
      )}
      {...props}
    />
  )
}

export { Input }
