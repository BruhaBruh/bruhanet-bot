import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export function Table({ className, ...props }: ComponentProps<'table'>) {
  return (
    <div className="overflow-x-auto">
      <table
        className={cn('admin-table w-full border-separate border-spacing-0 tracking-[-0.1px]', className)}
        {...props}
      />
    </div>
  )
}

export const TableHeader = (props: ComponentProps<'thead'>) => <thead {...props} />

export function TableBody({ className, ...props }: ComponentProps<'tbody'>) {
  return <tbody className={cn('[&_tr:last-child_td]:shadow-none', className)} {...props} />
}

export const TableRow = (props: ComponentProps<'tr'>) => <tr {...props} />

export function TableHead({ className, ...props }: ComponentProps<'th'>) {
  return (
    <th
      className={cn(
        'caption-1 relative bg-[color-mix(in_srgb,var(--page-bg)_55%,var(--surface))] px-3 py-2.5 align-middle font-semibold tracking-[0.2px] whitespace-nowrap text-text-secondary uppercase shadow-[inset_0_-0.5px_0_var(--separator)] first:pl-5 last:pr-5',
        className,
      )}
      {...props}
    />
  )
}

export function TableCell({ className, ...props }: ComponentProps<'td'>) {
  return (
    <td
      className={cn(
        'h-13 px-3 align-middle shadow-[inset_0_-0.5px_0_var(--separator)] first:pl-5 last:pr-5',
        className,
      )}
      {...props}
    />
  )
}
