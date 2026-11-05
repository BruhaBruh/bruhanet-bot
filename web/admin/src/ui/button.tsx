import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'
import { Icon, type IconName } from './icon'
import { Button as ButtonPrimitive } from './primitives/button'
import { Spinner } from './spinner'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'filled' | 'tinted' | 'gray' | 'plain' | 'destructive' | 'destructive-filled'
  size?: 'lg' | 'md' | 'sm'
  block?: boolean
  loading?: boolean
  icon?: IconName
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'filled', size = 'md', block, loading, icon, disabled, type = 'button', className, children, ...props },
  ref,
) {
  const glyph = size === 'sm' ? 16 : 20
  return (
    <ButtonPrimitive
      ref={ref}
      variant={variant}
      size={size}
      block={block}
      type={type}
      disabled={disabled || loading}
      className={cn(loading && 'disabled:opacity-100', className)}
      {...props}
    >
      {loading ? (
        <Spinner size={glyph} color="currentColor" />
      ) : (
        <>
          {icon && <Icon name={icon} size={glyph} strokeWidth={2} />}
          {children != null && <span>{children}</span>}
        </>
      )}
    </ButtonPrimitive>
  )
})
