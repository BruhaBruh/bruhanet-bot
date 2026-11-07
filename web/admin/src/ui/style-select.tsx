import { Select as SelectPrimitive } from 'radix-ui'
import { cn } from '@/lib/utils'
import { Icon } from './icon'
import { Tip } from './tip'

import { STYLE_OPTIONS } from './style-options'

const Dot = ({ value }: { value: string }) => (
  <span aria-hidden className={cn('size-3.5 flex-none rounded-full', (STYLE_OPTIONS.find((o) => o.value === value) ?? STYLE_OPTIONS[0]).dot)} />
)

export interface StyleSelectProps {
  value: string
  onChange(value: string): void
  disabled?: boolean
  'aria-label': string
  className?: string
}

/** Компактный выбор стиля кнопки: на триггере только цветная точка, в списке точка и короткая подпись. */
export function StyleSelect({ value, onChange, disabled, className, ...props }: StyleSelectProps) {
  // Radix не принимает пустое значение пункта: '' кодируем как 'default'.
  const enc = (v: string) => v || 'default'
  return (
    <SelectPrimitive.Root value={enc(value)} onValueChange={(v) => onChange(v === 'default' ? '' : v)} disabled={disabled}>
      <Tip label={STYLE_OPTIONS.find((o) => o.value === value)?.label}>
        <SelectPrimitive.Trigger
          aria-label={props['aria-label']}
          className={cn(
            'focus-ring inline-flex h-(--size-cell) flex-none cursor-pointer items-center gap-1.5 rounded-md bg-[var(--field-bg,var(--surface))] px-3 disabled:cursor-default disabled:opacity-40',
            className,
          )}
        >
          <Dot value={value} />
          <Icon name="chevron-down" size={14} strokeWidth={2} className="text-text-secondary" />
        </SelectPrimitive.Trigger>
      </Tip>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content position="popper" sideOffset={6} align="end" className="admin-table z-50 flex min-w-40 flex-col rounded-lg bg-surface-elevated p-1 shadow-popover">
          <SelectPrimitive.Viewport>
            {STYLE_OPTIONS.map((o) => (
              <SelectPrimitive.Item
                key={o.value}
                value={enc(o.value)}
                className="flex cursor-pointer items-center gap-2.5 rounded-sm px-3 py-2 text-text outline-none data-[highlighted]:bg-cell-highlight"
              >
                <Dot value={o.value} />
                <SelectPrimitive.ItemText>{o.label}</SelectPrimitive.ItemText>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}
