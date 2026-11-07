import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Icon } from './icon'
import { tgEntities } from './tg-entities'

export interface KeyboardButton {
  text: string
  url?: string
  style?: 'default' | 'primary' | 'success' | 'danger'
  onClick?: () => void
  /** Что положить внутрь кнопки (например, точку привязки стрелки на схеме); кнопка уже `relative`. */
  handle?: ReactNode
}

export interface MessageBubbleProps {
  out?: boolean
  time?: string
  edited?: boolean
  author?: ReactNode
  media?: ReactNode
  tail?: boolean
  keyboard?: KeyboardButton[][]
  className?: string
  children?: ReactNode
}

const KEYBOARD: Record<NonNullable<KeyboardButton['style']>, string> = {
  default: 'bg-keyboard-btn-bg text-accent-strong backdrop-blur-[10px]',
  primary: 'bg-accent-strong text-on-accent-strong',
  success: 'bg-success-strong text-on-success-strong',
  danger: 'bg-destructive-strong text-on-destructive-strong',
}

export function MessageBubble({
  out,
  time,
  edited,
  author,
  media,
  tail = true,
  keyboard = [],
  className,
  children,
}: MessageBubbleProps) {
  return (
    <div className={cn('flex max-w-85 flex-col gap-1', out ? 'ml-auto items-end' : 'items-start', className)}>
      <div
        className={cn(
          'callout max-w-full rounded-bubble py-1.5 pr-2.5 pl-3 text-text shadow-[0_1px_1px_rgba(0,0,0,0.08)]',
          out ? 'bg-bubble-out' : 'bg-bubble-in',
          tail && (out ? 'rounded-br-xs' : 'rounded-bl-xs'),
        )}
      >
        {author && <div className="admin-table leading-4.5 font-semibold text-accent-strong">{author}</div>}
        {media && <div className="-mt-0.5 -mr-1.5 mb-1.5 -ml-2 overflow-hidden rounded-xl">{media}</div>}
        <div className={tgEntities}>
          {children}
          <span
            className={cn(
              'caption-1 relative top-0.5 float-right mt-1.5 -mb-1 ml-2.5 inline-flex items-center gap-0.5',
              out ? 'text-bubble-meta' : 'text-text-hint',
            )}
          >
            {edited && 'изм. '}
            {time}
            {out && <Icon name="check" size={14} strokeWidth={2.4} />}
          </span>
        </div>
      </div>
      {keyboard.length > 0 && (
        <div className="flex w-full flex-col gap-1">
          {keyboard.map((row, i) => (
            <div key={i} className="flex gap-1">
              {row.map((button, j) => (
                <button
                  key={j}
                  type="button"
                  className={cn(
                    'focus-ring relative h-10 min-w-0 flex-1 cursor-pointer rounded-sm px-3 text-[14px] font-semibold tracking-[-0.1px]',
                    KEYBOARD[button.style ?? 'default'],
                  )}
                  onClick={button.onClick}
                >
                  <span className="block truncate">{button.text}</span>
                  {button.handle}
                  {button.url && (
                    <Icon name="link" size={12} strokeWidth={2.4} className="absolute top-1 right-1" />
                  )}
                </button>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
