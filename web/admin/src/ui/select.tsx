import { Select as SelectPrimitive } from 'radix-ui'
import { cn } from '@/lib/utils'
import { Icon } from './icon'

export interface SelectOption {
  value: string
  label: string
}

export interface SelectProps {
  value: string
  options: SelectOption[]
  onChange(value: string): void
  /** Что показать, пока значение не выбрано (value = ''). */
  placeholder?: string
  disabled?: boolean
  'aria-label': string
  className?: string
}

/** Выбор из списка в стиле `StyleSelect`: поле как у Input, список-поповер вместо системного. Значения пунктов непустые. */
export function Select({ value, options, onChange, placeholder, disabled, className, ...props }: SelectProps) {
  return (
    <SelectPrimitive.Root value={value || undefined} onValueChange={onChange} disabled={disabled}>
      <SelectPrimitive.Trigger
        aria-label={props['aria-label']}
        className={cn(
          'focus-ring admin-body inline-flex h-(--size-cell) min-w-0 cursor-pointer items-center justify-between gap-2 rounded-md bg-[var(--field-bg,var(--surface))] px-3 text-left text-text disabled:cursor-default disabled:opacity-40',
          className,
        )}
      >
        <span className={cn('min-w-0 truncate', !value && 'text-text-hint')}>
          <SelectPrimitive.Value placeholder={placeholder} />
        </span>
        <Icon name="chevron-down" size={14} strokeWidth={2} className="flex-none text-text-secondary" />
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content position="popper" sideOffset={6} align="start" className="admin-table z-50 flex max-h-72 min-w-(--radix-select-trigger-width) flex-col rounded-lg bg-surface-elevated p-1 shadow-popover">
          <SelectPrimitive.Viewport>
            {options.map((o) => (
              <SelectPrimitive.Item key={o.value} value={o.value} className="flex cursor-pointer items-center rounded-sm px-3 py-2 text-text outline-none data-[highlighted]:bg-cell-highlight">
                <SelectPrimitive.ItemText>{o.label}</SelectPrimitive.ItemText>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}
