import type { KeyboardEvent, ReactNode } from 'react'
import { Link } from 'react-router'
import { cn } from '@/lib/utils'
import { Icon } from './icon'

export interface CellProps {
  title: ReactNode
  subtitle?: ReactNode
  value?: ReactNode
  /** IconTile, Avatar, Checkbox. */
  before?: ReactNode
  /** Switch, Badge, StatusTag, Button size="sm". Не класть интерактивное в кликабельную Cell: кнопка в кнопке. */
  after?: ReactNode
  chevron?: boolean
  destructive?: boolean
  accent?: boolean
  onClick?: () => void
  /** Внутренний путь (`/users/5`) — Link роутера, остальное — обычная ссылка. */
  href?: string
  className?: string
}

const isInternal = (href: string) => href.startsWith('/') && !href.startsWith('//')

// У ссылки браузер сам обрабатывает только Enter.
function spaceClicks(e: KeyboardEvent<HTMLAnchorElement>) {
  if (e.key === ' ') {
    e.preventDefault()
    e.currentTarget.click()
  }
}

export function Cell({
  title,
  subtitle,
  value,
  before,
  after,
  chevron,
  destructive,
  accent,
  onClick,
  href,
  className,
}: CellProps) {
  const root = cn(
    'group body flex min-h-(--size-cell) w-full items-center bg-surface pl-4 text-left text-text no-underline',
    (onClick || href || chevron) &&
      'focus-ring cursor-pointer focus-visible:-outline-offset-2 hover:bg-[color-mix(in_srgb,var(--cell-highlight)_45%,var(--surface))] active:bg-cell-highlight',
    className,
  )
  const content = (
    <>
      {before && <span className="mr-3 flex flex-none items-center">{before}</span>}
      <span
        className={cn(
          'flex min-w-0 flex-1 items-center gap-2 self-stretch py-2.75 pr-4 shadow-[inset_0_0.5px_0_var(--separator)] group-first:shadow-none',
          subtitle && 'min-h-(--size-cell-subtitle) py-2',
        )}
      >
        <span className="flex min-w-0 flex-1 flex-col">
          <span
            className={cn('truncate', destructive && 'text-destructive', accent && 'text-accent-text')}
          >
            {title}
          </span>
          {subtitle && <span className="subheadline truncate text-text-secondary">{subtitle}</span>}
        </span>
        {value != null && <span className="whitespace-nowrap text-text-secondary">{value}</span>}
        {after && <span className="flex flex-none items-center gap-2">{after}</span>}
        {chevron && (
          <Icon name="chevron-right" size={18} strokeWidth={2.2} className="-mr-1 text-text-hint opacity-60" />
        )}
      </span>
    </>
  )

  if (href !== undefined) {
    return isInternal(href) ? (
      <Link to={href} className={root} onClick={onClick} onKeyDown={spaceClicks}>
        {content}
      </Link>
    ) : (
      <a href={href} className={root} onClick={onClick} onKeyDown={spaceClicks}>
        {content}
      </a>
    )
  }
  if (onClick) {
    return (
      <button type="button" className={root} onClick={onClick}>
        {content}
      </button>
    )
  }
  return <div className={root}>{content}</div>
}
