import { forwardRef, type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className, ...props },
  ref,
) {
  return (
    <input
      ref={ref}
      className={cn(
        'min-w-0 flex-1 bg-transparent p-0 text-text outline-none placeholder:text-text-hint',
        className,
      )}
      {...props}
    />
  )
})
