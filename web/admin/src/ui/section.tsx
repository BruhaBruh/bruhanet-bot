import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface SectionProps {
  header?: ReactNode
  footer?: ReactNode
  plain?: boolean
  className?: string
  children?: ReactNode
}

export function Section({ header, footer, plain, className, children }: SectionProps) {
  return (
    <section className={cn('mx-4 mb-8', plain && 'mx-0', className)}>
      {header && <h3 className="footnote px-4 pb-1.75 text-section-header uppercase">{header}</h3>}
      <div className={cn('overflow-hidden rounded-md bg-surface', plain && 'rounded-none')}>{children}</div>
      {footer && <p className="footnote px-4 pt-1.75 text-section-header">{footer}</p>}
    </section>
  )
}
