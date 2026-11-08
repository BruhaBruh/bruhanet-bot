import { useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { AlertDialog, AlertDialogContent, AlertDialogDescription, AlertDialogTitle } from './primitives/alert-dialog'

export interface DialogAction {
  label: string
  style?: 'default' | 'cancel' | 'destructive'
  onClick?: () => void
}

export interface DialogProps {
  title?: ReactNode
  message?: ReactNode
  actions?: DialogAction[]
  className?: string
  children?: ReactNode
}

const DEFAULT_ACTIONS: DialogAction[] = [{ label: 'OK' }]

/**
 * Алерт iOS, открыт, пока смонтирован: `{open && <Dialog … />}`; закрывает вызывающий в onClick действий.
 * Esc равен действию `cancel`, без него не закрывается. Кнопки обычные, а не Radix Action/Cancel:
 * те дёргают onOpenChange и вызвали бы onClick действия дважды.
 */
export function Dialog({ title, message, actions = DEFAULT_ACTIONS, className, children }: DialogProps) {
  const [opener] = useState(() => document.activeElement as HTMLElement | null)
  const row = actions.length === 2
  const cancel = actions.find((a) => a.style === 'cancel')

  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open) cancel?.onClick?.()
      }}
    >
      <AlertDialogContent
        onOpenAutoFocus={(e) => {
          // Фокус на безопасном действии: случайный Enter не должен подтвердить удаление.
          e.preventDefault()
          const buttons = (e.target as HTMLElement).querySelectorAll('button')
          buttons[cancel ? actions.indexOf(cancel) : 0]?.focus()
        }}
        onCloseAutoFocus={(e) => {
          e.preventDefault()
          opener?.focus()
        }}
        className={cn(
          'w-67.5 overflow-hidden rounded-xl bg-surface-elevated text-center text-text shadow-popover',
          className,
        )}
      >
        <div className="px-4 pt-4.75 pb-4">
          {title ? (
            <AlertDialogTitle className="headline">{title}</AlertDialogTitle>
          ) : (
            <AlertDialogTitle className="sr-only">Подтверждение</AlertDialogTitle>
          )}
          {/* Radix требует описание у alertdialog: без сообщения — пустое и скрытое. */}
          <AlertDialogDescription className={message ? 'footnote mt-0.5 text-text' : 'sr-only'}>
            {message}
          </AlertDialogDescription>
          {children}
        </div>
        <div className={cn('body flex flex-col shadow-[inset_0_0.5px_0_var(--separator)]', row && 'flex-row')}>
          {actions.map((action, i) => (
            <button
              key={action.label}
              type="button"
              className={cn(
                'focus-ring min-h-11 flex-1 cursor-pointer text-accent-text focus-visible:-outline-offset-2 active:bg-cell-highlight',
                action.style === 'cancel' && 'font-semibold',
                action.style === 'destructive' && 'text-destructive',
                i > 0 &&
                  (row ? 'shadow-[inset_0.5px_0_0_var(--separator)]' : 'shadow-[inset_0_0.5px_0_var(--separator)]'),
              )}
              onClick={action.onClick}
            >
              {action.label}
            </button>
          ))}
        </div>
      </AlertDialogContent>
    </AlertDialog>
  )
}
