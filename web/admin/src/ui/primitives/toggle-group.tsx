import { ToggleGroup as ToggleGroupPrimitive } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export function ToggleGroup({ className, ...props }: ComponentProps<typeof ToggleGroupPrimitive.Root>) {
  return (
    <ToggleGroupPrimitive.Root
      className={cn('inline-flex gap-0.5 rounded-[9px] bg-fill-tertiary p-0.5', className)}
      {...props}
    />
  )
}

export function ToggleGroupItem({ className, ...props }: ComponentProps<typeof ToggleGroupPrimitive.Item>) {
  return (
    <ToggleGroupPrimitive.Item
      // #636366 — в токенах такого цвета нет, значение из bundle.css DS.
      className={cn(
        'focus-ring footnote h-7 cursor-pointer rounded-[7px] px-3 font-medium text-text data-[state=on]:bg-surface-elevated data-[state=on]:font-semibold data-[state=on]:shadow-thumb dark:data-[state=on]:bg-[#636366]',
        className,
      )}
      {...props}
    />
  )
}
