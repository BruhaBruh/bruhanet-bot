import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export type Tone = 'neutral' | 'accent' | 'success' | 'warning' | 'destructive'

const TONES: Record<Tone, string> = {
  neutral: 'bg-fill-tertiary text-text-secondary',
  accent: 'bg-accent-tint text-accent-strong',
  success: 'bg-success-tint text-success-strong',
  warning: 'bg-warning-tint text-warning-strong',
  destructive: 'bg-destructive-tint text-destructive-strong',
}

export interface StatusTagProps {
  tone?: Tone
  dot?: boolean
  className?: string
  children?: ReactNode
}

export function StatusTag({ tone = 'neutral', dot, className, children }: StatusTagProps) {
  return (
    <span
      className={cn(
        'caption-1 inline-flex h-5.5 items-center gap-1 rounded-xs px-2 font-semibold whitespace-nowrap',
        TONES[tone],
        className,
      )}
    >
      {dot && <span className="size-1.5 rounded-full bg-current" />}
      {children}
    </span>
  )
}
