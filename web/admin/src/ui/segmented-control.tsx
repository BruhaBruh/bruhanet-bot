import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { ToggleGroup, ToggleGroupItem } from './primitives/toggle-group'

export interface SegmentedControlProps {
  options: Array<string | { value: string; label: ReactNode }>
  value?: string
  onChange?: (value: string) => void
  block?: boolean
  className?: string
}

export function SegmentedControl({ options, value, onChange, block, className }: SegmentedControlProps) {
  return (
    <ToggleGroup
      type="single"
      value={value ?? ''}
      // Radix отдаёт '' при клике по активному сегменту: выбор всегда один.
      onValueChange={(next) => {
        if (next) onChange?.(next)
      }}
      className={cn(block && 'flex', className)}
    >
      {options.map((option) => {
        const { value: v, label } = typeof option === 'string' ? { value: option, label: option } : option
        return (
          <ToggleGroupItem key={v} value={v} className={cn(block && 'flex-1')}>
            {label}
          </ToggleGroupItem>
        )
      })}
    </ToggleGroup>
  )
}
