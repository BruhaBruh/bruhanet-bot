import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { Icon, type IconName } from './icon'
import { Tip } from './tip'

export interface RowActionProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: IconName
  label: string
}

/** Кнопка-иконка в `rowActions` таблицы. */
export const RowAction = forwardRef<HTMLButtonElement, RowActionProps>(function RowAction({ icon, label, ...props }, ref) {
  return (
    <Tip label={label}>
      <button
        ref={ref}
        type="button"
        aria-label={label}
        className="focus-ring inline-flex size-8 cursor-pointer items-center justify-center rounded-sm text-text-secondary hover:bg-fill-tertiary hover:text-text"
        {...props}
      >
        <Icon name={icon} size={18} />
      </button>
    </Tip>
  )
})
