import { forwardRef, useId, type TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'
import { Field, fieldBox } from './input'
import { Textarea as TextareaPrimitive } from './primitives/textarea'

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string
  hint?: string
  error?: string
  showCount?: boolean
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { label, hint, error, showCount, className, id, rows = 4, ...props },
  ref,
) {
  const autoId = useId()
  const fieldId = id ?? autoId
  const { maxLength, value } = props
  return (
    <Field id={fieldId} label={label} hint={hint} error={error} className={className}>
      <div className={cn(fieldBox, 'flex-col items-stretch pt-2.75 pb-2', error && 'border-destructive focus-within:border-destructive')}>
        <TextareaPrimitive
          ref={ref}
          id={fieldId}
          rows={rows}
          aria-invalid={error ? true : undefined}
          aria-describedby={error || hint ? `${fieldId}-msg` : undefined}
          {...props}
        />
        {maxLength && showCount !== false && (
          <span className="caption-1 self-end text-text-secondary">
            {String(value ?? '').length} / {maxLength}
          </span>
        )}
      </div>
    </Field>
  )
})
