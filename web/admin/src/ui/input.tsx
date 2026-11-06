import { forwardRef, useId, type InputHTMLAttributes, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Input as InputPrimitive } from './primitives/input'

interface FieldProps {
  id: string
  label?: string
  hint?: string
  error?: string
  className?: string
  children: ReactNode
}

export function Field({ id, label, hint, error, className, children }: FieldProps) {
  const message = error ?? hint
  return (
    <div className={cn('body flex flex-col gap-1.5', className)}>
      {label && (
        <label className="admin-label px-0.5 text-text-secondary" htmlFor={id}>
          {label}
        </label>
      )}
      {children}
      {message && (
        <p
          id={`${id}-msg`}
          className={cn('footnote px-0.5', error ? 'text-destructive-strong' : 'text-text-secondary')}
        >
          {message}
        </p>
      )}
    </div>
  )
}

export const fieldBox =
  'flex gap-2 rounded-md border border-transparent bg-[var(--field-bg,var(--surface))] px-4 transition-[border-color] duration-150 focus-within:border-accent has-[:disabled]:opacity-40'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string
  hint?: string
  error?: string
  before?: ReactNode
  after?: ReactNode
  /** Ширина по содержимому (до max-w-40) для одиночного поля; в сетке поле на всю ячейку. */
  fit?: boolean
}

const addon = 'flex items-center text-text-secondary'

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, hint, error, before, after, fit, className, id, ...props },
  ref,
) {
  const autoId = useId()
  const fieldId = id ?? autoId
  // ch, а не field-sizing: Firefox его не умеет.
  const chars = Math.max(String(props.value ?? '').length || (props.placeholder?.length ?? 0), 3)
  return (
    <Field id={fieldId} label={label} hint={hint} error={error} className={className}>
      <div
        className={cn(
          fieldBox,
          'min-h-(--size-cell) items-center',
          fit && 'w-fit max-w-40',
          error && 'border-destructive focus-within:border-destructive',
        )}
      >
        {before && <span className={addon}>{before}</span>}
        <InputPrimitive
          ref={ref}
          id={fieldId}
          aria-invalid={error ? true : undefined}
          aria-describedby={error || hint ? `${fieldId}-msg` : undefined}
          {...props}
          className={cn(fit && 'flex-initial', props.type === 'number' && 'text-right tabular-nums')}
          style={fit ? { width: `${chars}ch` } : undefined}
        />
        {after && <span className={addon}>{after}</span>}
      </div>
    </Field>
  )
})
