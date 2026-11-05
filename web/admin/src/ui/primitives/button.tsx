import { cva, type VariantProps } from 'class-variance-authority'
import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

// eslint-disable-next-line react-refresh/only-export-components -- для ссылок в виде кнопки
export const buttonVariants = cva(
  'focus-ring inline-flex cursor-pointer items-center justify-center gap-2 font-semibold whitespace-nowrap transition-[opacity,background-color] duration-150 active:opacity-70 disabled:cursor-default disabled:opacity-40',
  {
    variants: {
      variant: {
        filled: 'bg-accent text-on-accent',
        tinted: 'bg-accent-tint text-accent-strong',
        gray: 'bg-fill-tertiary text-text',
        plain: 'bg-transparent text-accent-text',
        destructive: 'bg-destructive-tint text-destructive-strong',
        'destructive-filled': 'bg-destructive text-white',
      },
      size: {
        lg: 'headline h-(--size-main-button) rounded-lg px-5',
        md: 'subheadline h-(--size-button-md) rounded-md px-4',
        sm: 'admin-table h-(--size-button-sm) rounded-full px-3 tracking-[-0.1px]',
      },
      block: { true: 'flex w-full' },
    },
    compoundVariants: [{ variant: 'plain', class: 'px-2' }],
    defaultVariants: { variant: 'filled', size: 'md' },
  },
)

export type ButtonVariantProps = VariantProps<typeof buttonVariants>

export const Button = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement> & ButtonVariantProps>(
  function Button({ className, variant, size, block, ...props }, ref) {
    return <button ref={ref} className={cn(buttonVariants({ variant, size, block }), className)} {...props} />
  },
)
