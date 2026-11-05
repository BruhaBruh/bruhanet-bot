import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Checkbox as CheckboxPrimitive } from './primitives/checkbox'

export interface CheckboxProps {
  checked?: boolean
  onChange?: (checked: boolean) => void
  disabled?: boolean
  label?: ReactNode
  className?: string
}

export function Checkbox({ checked, onChange, disabled, label, className }: CheckboxProps) {
  return (
    <label
      className={cn(
        'relative inline-flex cursor-pointer items-center gap-3',
        disabled && 'cursor-default opacity-40',
        className,
      )}
    >
      <CheckboxPrimitive
        checked={!!checked}
        disabled={disabled}
        onCheckedChange={(next) => onChange?.(next === true)}
      />
      {label && <span>{label}</span>}
    </label>
  )
}
