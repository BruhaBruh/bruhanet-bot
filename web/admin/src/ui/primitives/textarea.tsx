import { forwardRef, type TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, ...props }, ref) {
    return (
      <textarea
        ref={ref}
        className={cn(
          'min-h-16.5 min-w-0 flex-1 resize-y bg-transparent p-0 text-text outline-none placeholder:text-text-hint',
          className,
        )}
        {...props}
      />
    )
  },
)
