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
  // block на телефоне переносит подписи: иначе длинные обрезаются краем прокрутки. Без block — прокрутка.
  return (
    <ToggleGroup
      type="single"
      value={value ?? ''}
      // Radix отдаёт '' при клике по активному сегменту: выбор всегда один.
      onValueChange={(next) => {
        if (next) onChange?.(next)
      }}
      className={cn('max-w-full overflow-x-auto', block && 'flex', className)}
    >
      {options.map((option) => {
        const { value: v, label } = typeof option === 'string' ? { value: option, label: option } : option
        return (
          <ToggleGroupItem key={v} value={v} className={cn('flex-none whitespace-nowrap', block && 'flex-1 max-md:h-auto max-md:min-h-7 max-md:px-2 max-md:py-1 max-md:whitespace-normal')}>
            {label}
          </ToggleGroupItem>
        )
      })}
    </ToggleGroup>
  )
}
