import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Icon } from './icon'

export interface NavBarProps {
  title?: ReactNode
  subtitle?: ReactNode
  back?: boolean | string
  onBack?: () => void
  left?: ReactNode
  right?: ReactNode
  large?: boolean
  className?: string
}

export function NavBar({ title, subtitle, back, onBack, left, right, large, className }: NavBarProps) {
  return (
    <header
      className={cn(
        'body bg-header-bg text-text backdrop-blur-[20px] backdrop-saturate-[180%]',
        !large && 'shadow-[inset_0_-0.5px_0_var(--separator)]',
        className,
      )}
    >
      <div className="grid h-(--size-nav-bar) grid-cols-[1fr_auto_1fr] items-center px-2">
        <div className="flex min-w-0 items-center gap-2">
          {back ? (
            <button
              type="button"
              className="focus-ring -ml-1 inline-flex cursor-pointer items-center pr-1 text-accent-text"
              onClick={onBack}
            >
              <Icon name="chevron-left" size={24} strokeWidth={2.4} />
              <span>{typeof back === 'string' ? back : 'Назад'}</span>
            </button>
          ) : (
            left
          )}
        </div>
        <div className="flex min-w-0 flex-col items-center text-center">
          {!large && (
            <>
              <span className="max-w-[60vw] truncate font-semibold">{title}</span>
              {subtitle && <span className="admin-label font-normal tracking-[-0.1px] text-text-secondary">{subtitle}</span>}
            </>
          )}
        </div>
        <div className="flex min-w-0 items-center justify-end gap-2">{right}</div>
      </div>
      {large && <h1 className="large-title px-4 pt-0.5 pb-2">{title}</h1>}
    </header>
  )
}
