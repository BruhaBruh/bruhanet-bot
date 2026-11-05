import { forwardRef, type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'
import { Icon } from './icon'
import { Input as InputPrimitive } from './primitives/input'

export interface SearchFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  onClear?: () => void
}

export const SearchField = forwardRef<HTMLInputElement, SearchFieldProps>(function SearchField(
  { onClear, className, placeholder = 'Поиск', 'aria-label': ariaLabel, ...props },
  ref,
) {
  return (
    <div
      className={cn(
        'body flex h-9 items-center gap-1.5 rounded-md bg-fill-tertiary px-2 text-text-secondary',
        className,
      )}
    >
      <Icon name="search" size={17} strokeWidth={2.2} />
      <InputPrimitive
        ref={ref}
        type="search"
        placeholder={placeholder}
        aria-label={ariaLabel ?? placeholder}
        className="placeholder:text-text-secondary [&::-webkit-search-cancel-button]:hidden"
        {...props}
      />
      {!!props.value && onClear && (
        <button
          type="button"
          className="focus-ring inline-flex size-4.25 cursor-pointer items-center justify-center rounded-full bg-text-hint text-surface"
          onClick={onClear}
          aria-label="Очистить"
        >
          <Icon name="close" size={12} strokeWidth={2.6} />
        </button>
      )}
    </div>
  )
})
