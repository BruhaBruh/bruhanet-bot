import { Fragment, type ReactElement, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Badge } from './badge'
import { Icon } from './icon'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './primitives/tooltip'
import type { NavItem } from './tab-bar'

export interface SidebarProps {
  items: Array<NavItem | { section: string }>
  value?: string
  onChange?: (key: string) => void
  /** Рейка 72px: подписи в подсказках и aria-label, секции — разделители, бейджи — точки. */
  collapsed?: boolean
  /** Рисует «Свернуть меню» внизу. */
  onToggle?: () => void
  header?: ReactNode
  footer?: ReactNode
  className?: string
}

const itemClass = (collapsed: boolean) =>
  cn(
    'focus-ring flex h-10 cursor-pointer items-center gap-3 rounded-sm px-3 text-left hover:bg-fill-tertiary',
    collapsed && 'w-12 justify-center self-center px-0',
  )

export function Sidebar({ items, value, onChange, collapsed = false, onToggle, header, footer, className }: SidebarProps) {
  const hinted = (label: string, button: ReactElement) =>
    collapsed ? (
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent side="right">{label}</TooltipContent>
      </Tooltip>
    ) : (
      button
    )
  const toggleLabel = collapsed ? 'Развернуть меню' : 'Свернуть меню'

  return (
    <TooltipProvider delayDuration={200}>
      <aside
        className={cn(
          'subheadline flex w-(--size-sidebar) flex-col gap-2 bg-surface px-2 py-3 text-text shadow-[inset_-0.5px_0_0_var(--separator)] transition-[width] duration-200 motion-reduce:transition-none',
          collapsed && 'w-18',
          className,
        )}
      >
        {header && <div className="px-3 pt-2 pb-3">{header}</div>}
        <nav className="flex flex-col gap-0.5">
          {items.map((item) => {
            if ('section' in item) {
              return collapsed ? (
                <div key={item.section} role="separator" className="m-3 h-[0.5px] bg-separator" />
              ) : (
                <div key={item.section} className="admin-label px-3 pt-4 pb-1 text-text-secondary">
                  {item.section}
                </div>
              )
            }
            const on = item.key === value
            return (
              <Fragment key={item.key}>
                {hinted(
                  item.label,
                  <button
                    type="button"
                    aria-label={collapsed ? item.label : undefined}
                    aria-current={on ? 'page' : undefined}
                    className={cn(
                      itemClass(collapsed),
                      on && 'bg-accent font-semibold text-on-accent hover:bg-accent',
                    )}
                    onClick={() => onChange?.(item.key)}
                  >
                    <span className="relative inline-flex flex-none">
                      <Icon name={item.icon} size={20} className={on ? 'text-on-accent' : 'text-text-secondary'} />
                      {collapsed && !!item.badge && (
                        <Badge
                          dot
                          className={cn(
                            'absolute -top-0.5 -right-0.75 size-2.25 min-w-2.25 shadow-[0_0_0_2px_var(--surface)]',
                            on && 'bg-on-accent shadow-[0_0_0_2px_var(--accent)]',
                          )}
                        />
                      )}
                    </span>
                    {!collapsed && <span className="min-w-0 flex-1 truncate">{item.label}</span>}
                    {!collapsed && !!item.badge && (
                      <Badge count={item.badge} className={cn(on && 'bg-on-accent text-accent')} />
                    )}
                  </button>,
                )}
              </Fragment>
            )
          })}
        </nav>
        {onToggle &&
          hinted(
            toggleLabel,
            <button
              type="button"
              className={cn(itemClass(collapsed), 'mt-auto text-text-secondary')}
              onClick={onToggle}
              aria-label={toggleLabel}
              aria-expanded={!collapsed}
            >
              <span className="relative inline-flex flex-none">
                <Icon name="sidebar" size={20} />
              </span>
              {!collapsed && <span className="min-w-0 flex-1 truncate">Свернуть меню</span>}
            </button>,
          )}
        {footer && <div className="px-3 py-2">{footer}</div>}
      </aside>
    </TooltipProvider>
  )
}
