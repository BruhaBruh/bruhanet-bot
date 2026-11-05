import { useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { Icon, type IconName } from './icon'

export interface ToastProps {
  icon?: IconName
  action?: string
  onAction?: () => void
  className?: string
  children?: ReactNode
}

export function Toast({ icon, action, onAction, className, children }: ToastProps) {
  return (
    <div
      role="status"
      className={cn(
        'subheadline flex min-h-12 items-center gap-3 rounded-lg bg-toast-bg px-4 py-2.5 text-on-toast backdrop-blur-[20px]',
        className,
      )}
    >
      {icon && <Icon name={icon} size={22} strokeWidth={2} />}
      <span className="flex-1">{children}</span>
      {action && (
        // #6ab3ff — в токенах нет: тост тёмный в обеих темах, значение из bundle.css DS.
        <button
          type="button"
          className="focus-ring cursor-pointer font-semibold text-[#6ab3ff]"
          onClick={onAction}
        >
          {action}
        </button>
      )}
    </div>
  )
}

export type ToastOptions = Pick<ToastProps, 'icon' | 'action' | 'onAction'>
type Item = ToastOptions & { id: number; text: string }

const VISIBLE_MS = 2500

// ponytail: один тост на провайдер, новый заменяет старый. Очередь — если тосты начнут теряться.
let show: ((item: Item) => void) | null = null
let nextId = 0

/** Показать тост из любого места, провайдер должен быть смонтирован. */
// eslint-disable-next-line react-refresh/only-export-components -- API тоста живёт рядом с провайдером
export function toast(text: string, options?: ToastOptions) {
  show?.({ ...options, text, id: ++nextId })
}

// eslint-disable-next-line react-refresh/only-export-components -- см. toast
export const useToast = () => toast

export function ToastProvider({ children }: { children?: ReactNode }) {
  const [item, setItem] = useState<Item | null>(null)

  useEffect(() => {
    show = setItem
    return () => {
      show = null
    }
  }, [])

  useEffect(() => {
    if (!item) return
    const timer = setTimeout(() => setItem(null), VISIBLE_MS)
    return () => clearTimeout(timer)
  }, [item])

  return (
    <>
      {children}
      {createPortal(
        // Над TabBar на мобильном. Порталом в body: над модальными окнами.
        <div className="pointer-events-none fixed inset-x-4 bottom-[calc(var(--size-tab-bar)+var(--space-4))] z-60 flex justify-center md:bottom-6">
          {item && (
            <Toast
              key={item.id}
              icon={item.icon}
              action={item.action}
              onAction={() => {
                item.onAction?.()
                setItem(null)
              }}
              className="pointer-events-auto motion-safe:animate-[tg-toast-in_.2s_ease-out]"
            >
              {item.text}
            </Toast>
          )}
        </div>,
        document.body,
      )}
    </>
  )
}
