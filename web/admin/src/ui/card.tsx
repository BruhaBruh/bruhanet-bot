import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface CardProps {
  title?: string
  aside?: string
  className?: string
  children?: ReactNode
}

/** Карточка формы. Поля на её белом фоне не видны, поэтому внутри они красятся цветом страницы (`--field-bg`). */
export function Card({ title, aside, className, children }: CardProps) {
  return (
    <section className={cn('flex flex-col gap-3 rounded-lg bg-surface p-5 shadow-card [--field-bg:var(--page-bg)]', className)}>
      {title && (
        <div className="flex flex-wrap items-baseline justify-between gap-3">
          <h2 className="headline">{title}</h2>
          {aside && <span className="footnote text-text-secondary">{aside}</span>}
        </div>
      )}
      {children}
    </section>
  )
}
