import { forwardRef, useId, type SelectHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'
import { Field, fieldBox } from './input'
import { Icon } from './icon'

export interface SelectOption {
  value: string
  label: string
}

export interface SelectProps extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'onChange'> {
  options: SelectOption[]
  onChange?: (value: string) => void
  label?: string
  hint?: string
}

// Системный <select>: клавиатура и экранный диктор из коробки, оформление как у Input.
export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { options, onChange, label, hint, className, id, ...props },
  ref,
) {
  const autoId = useId()
  const fieldId = id ?? autoId
  return (
    <Field id={fieldId} label={label} hint={hint} className={className}>
      <div className={cn(fieldBox, 'relative min-h-(--size-cell) items-center')}>
        <select
          ref={ref}
          id={fieldId}
          className="min-w-0 flex-1 cursor-pointer appearance-none bg-transparent py-2 pr-6 text-text outline-none disabled:cursor-default"
          onChange={(e) => onChange?.(e.target.value)}
          {...props}
        >
          {options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        <Icon name="chevron-down" size={16} strokeWidth={2} className="pointer-events-none absolute right-4 text-text-secondary" />
      </div>
    </Field>
  )
})
