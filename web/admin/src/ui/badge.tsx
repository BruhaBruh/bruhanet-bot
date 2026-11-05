import { cn } from '@/lib/utils'

export interface BadgeProps {
  count?: number
  max?: number
  muted?: boolean
  dot?: boolean
  className?: string
}

export function Badge({ count, max, muted, dot, className }: BadgeProps) {
  if (count == null && !dot) return null
  const text = dot || count == null ? '' : max && count > max ? `${max}+` : String(count)
  return (
    <span
      data-dot={dot || undefined}
      className={cn(
        'inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-accent px-1.5 text-[13px] leading-5 font-semibold text-on-accent tabular-nums',
        muted && 'bg-text-hint text-surface',
        dot && 'h-2.5 w-2.5 min-w-2.5 p-0',
        className,
      )}
    >
      {text}
    </span>
  )
}
