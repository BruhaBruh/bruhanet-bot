import type { ReactNode } from 'react'
import { Button } from '@/ui/primitives/button'
import { Spinner } from '@/ui/spinner'

export function Loading() {
  return (
    <div className="flex min-h-[40dvh] items-center justify-center">
      <Spinner />
    </div>
  )
}

// Полноэкранное сообщение: сессия закончилась, юзера нет в боте, сбой загрузки.
export function Notice({ title, children, action }: { title: string; children?: ReactNode; action?: { label: string; onClick: () => void } }) {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-3 p-6 text-center">
      <h1 className="title-3 font-semibold">{title}</h1>
      {children && <p className="subheadline max-w-80 text-text-secondary">{children}</p>}
      {action && (
        <Button size="md" className="mt-2" onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </main>
  )
}
