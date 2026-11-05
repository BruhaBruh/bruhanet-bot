import { useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Icon } from './icon'
import { Dialog, DialogContent, DialogTitle } from './primitives/dialog'

export interface SheetProps {
  title?: ReactNode
  left?: ReactNode
  onClose?: () => void
  footer?: ReactNode
  className?: string
  children?: ReactNode
}

/** Открыт, пока смонтирован: `{open && <Sheet … />}`. На 768+ — модальное окно до 480px, ниже — лист снизу. */
export function Sheet({ title, left, onClose, footer, className, children }: SheetProps) {
  const [opener] = useState(() => document.activeElement as HTMLElement | null)
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose?.()
      }}
    >
      <DialogContent
        aria-describedby={undefined}
        onCloseAutoFocus={(e) => {
          e.preventDefault()
          opener?.focus()
        }}
        className={cn(
          'body inset-x-0 bottom-0 max-h-[90dvh] overflow-auto rounded-t-sheet bg-page-bg text-text shadow-popover md:inset-x-auto md:top-1/2 md:bottom-auto md:left-1/2 md:w-full md:max-w-120 md:-translate-x-1/2 md:-translate-y-1/2 md:rounded-sheet',
          className,
        )}
      >
        <div className="mx-auto mt-1.5 h-1.25 w-9 rounded-[3px] bg-text-hint opacity-40" />
        {title || onClose ? (
          <div className="grid h-12 grid-cols-[1fr_auto_1fr] items-center px-4">
            <span className="flex items-center">{left}</span>
            <DialogTitle className="headline text-center">{title}</DialogTitle>
            <span className="flex items-center justify-end">
              {onClose && (
                <button
                  type="button"
                  className="focus-ring inline-flex size-(--size-button-sm) cursor-pointer items-center justify-center rounded-full bg-fill-tertiary text-text-secondary"
                  onClick={onClose}
                  aria-label="Закрыть"
                >
                  <Icon name="close" size={14} strokeWidth={2.6} />
                </button>
              )}
            </span>
          </div>
        ) : (
          <DialogTitle className="sr-only">Окно</DialogTitle>
        )}
        <div className="pt-2 pb-4">{children}</div>
        {footer && <div className="px-4 pt-2 pb-4">{footer}</div>}
      </DialogContent>
    </Dialog>
  )
}
