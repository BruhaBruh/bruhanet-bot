import { Checkbox as CheckboxPrimitive } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'
import { Icon } from '../icon'

export function Checkbox({ className, ...props }: ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      className={cn(
        'focus-ring inline-flex size-5.5 flex-none cursor-pointer items-center justify-center rounded-full border-[1.5px] border-text-hint text-on-accent transition-[background-color,border-color] duration-150 disabled:cursor-default data-[state=checked]:border-accent data-[state=checked]:bg-accent',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator className="inline-flex">
        <Icon name="check" size={14} strokeWidth={3} />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}
