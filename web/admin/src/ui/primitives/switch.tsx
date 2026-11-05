import { Switch as SwitchPrimitive } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export function Switch({ className, ...props }: ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      className={cn(
        'focus-ring relative h-6 w-10 flex-none cursor-pointer rounded-full bg-switch-off p-0.5 transition-[background-color] duration-200 disabled:cursor-default disabled:opacity-40 data-[state=checked]:bg-switch-on',
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb className="block size-5 rounded-full bg-white shadow-thumb transition-transform duration-200 data-[state=checked]:translate-x-4" />
    </SwitchPrimitive.Root>
  )
}
