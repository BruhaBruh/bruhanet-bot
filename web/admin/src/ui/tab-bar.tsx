import { cn } from '@/lib/utils'
import { Badge } from './badge'
import { Icon, type IconName } from './icon'

export interface NavItem {
  key: string
  label: string
  icon: IconName
  badge?: number
}

export interface TabBarProps {
  items: NavItem[]
  value?: string
  onChange?: (key: string) => void
  className?: string
}

export function TabBar({ items, value, onChange, className }: TabBarProps) {
  return (
    <nav
      // Прокрутка вместо сжатия, когда вкладок больше, чем влезает в 390px (решение плана №8).
      className={cn(
        'flex h-(--size-tab-bar) overflow-x-auto bg-bottom-bar-bg shadow-[inset_0_0.5px_0_var(--separator)] backdrop-blur-[20px] backdrop-saturate-[180%] [scrollbar-width:none]',
        className,
      )}
    >
      {items.map((item) => {
        const on = item.key === value
        return (
          <button
            key={item.key}
            type="button"
            aria-current={on ? 'page' : undefined}
            className={cn(
              'focus-ring flex min-w-16 focus-visible:-outline-offset-2 flex-1 shrink-0 cursor-pointer flex-col items-center justify-center gap-px pt-1 text-text-hint',
              on && 'text-accent-strong',
            )}
            onClick={() => onChange?.(item.key)}
          >
            <span className="relative inline-flex">
              <Icon name={item.icon} size={26} strokeWidth={on ? 2.1 : 1.7} />
              {!!item.badge && (
                <Badge
                  count={item.badge}
                  className="absolute -top-0.75 left-4.25 h-4.5 min-w-4.5 px-1.25 text-[12px] leading-4.5 shadow-[0_0_0_2px_var(--bottom-bar-bg)]"
                />
              )}
            </span>
            <span className="caption-2">{item.label}</span>
          </button>
        )
      })}
    </nav>
  )
}
