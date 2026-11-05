import { DropdownMenu } from 'radix-ui'
import { useRef } from 'react'
import { cn } from '@/lib/utils'
import { RowAction } from './row-action'

export interface RowMenuItem {
  label: string
  destructive?: boolean
  disabled?: boolean
  onSelect: () => void
}

export interface RowMenuProps {
  label: string
  items: RowMenuItem[]
}

/** Кнопка `more` с меню в `rowActions` таблицы. */
export function RowMenu({ label, items }: RowMenuProps) {
  const trigger = useRef<HTMLButtonElement>(null)
  return (
    // modal={false}: модальное меню оставляет body без pointer-events, если пункт открывает Dialog.
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild>
        <RowAction ref={trigger} icon="more" label={label} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="admin-table z-50 flex min-w-40 flex-col rounded-lg bg-surface-elevated p-1 text-text shadow-popover"
        >
          {items.map((item) => (
            <DropdownMenu.Item
              key={item.label}
              disabled={item.disabled}
              className={cn(
                'flex h-9 cursor-pointer items-center rounded-sm px-3 outline-none data-disabled:cursor-default data-disabled:opacity-40 data-highlighted:bg-cell-highlight',
                item.destructive && 'text-destructive-strong',
              )}
              onSelect={() => {
                // Dialog запоминает фокус при монтировании и вернёт его сюда, а не на исчезающий пункт.
                trigger.current?.focus()
                item.onSelect()
              }}
            >
              {item.label}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
