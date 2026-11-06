import { RadioGroup as RadioGroupPrimitive } from 'radix-ui'
import { useId, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface RadioGroupProps {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  'aria-label': string
  className?: string
  children: ReactNode
}

export function RadioGroup({ value, onChange, className, children, ...props }: RadioGroupProps) {
  return (
    <RadioGroupPrimitive.Root value={value} onValueChange={onChange} className={cn('flex flex-col', className)} {...props}>
      {children}
    </RadioGroupPrimitive.Root>
  )
}

export interface RadioProps {
  value: string
  label: ReactNode
  /** Справа от подписи: счётчик, поле. */
  after?: ReactNode
  disabled?: boolean
  className?: string
}

// Круглый индикатор как у Checkbox; подпись связана с кнопкой через label.
export function Radio({ value, label, after, disabled, className }: RadioProps) {
  const id = `${useId()}-${value}`
  return (
    <div className={cn('flex min-h-11 items-center gap-3', className)}>
      <RadioGroupPrimitive.Item
        id={id}
        value={value}
        disabled={disabled}
        className="focus-ring inline-flex size-5.5 flex-none cursor-pointer items-center justify-center rounded-full border-[1.5px] border-text-hint transition-[border-color] duration-150 disabled:cursor-default disabled:opacity-40 data-[state=checked]:border-accent"
      >
        <RadioGroupPrimitive.Indicator className="size-3 rounded-full bg-accent" />
      </RadioGroupPrimitive.Item>
      <label htmlFor={id} className={cn('grow cursor-pointer', disabled && 'cursor-default opacity-40')}>
        {label}
      </label>
      {after}
    </div>
  )
}
