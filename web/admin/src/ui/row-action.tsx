import { Icon, type IconName } from './icon'

export interface RowActionProps {
  icon: IconName
  label: string
  onClick?: () => void
}

/** Кнопка-иконка в `rowActions` таблицы. */
export function RowAction({ icon, label, onClick }: RowActionProps) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className="focus-ring inline-flex size-8 cursor-pointer items-center justify-center rounded-sm text-text-secondary hover:bg-fill-tertiary hover:text-text"
      onClick={onClick}
    >
      <Icon name={icon} size={18} />
    </button>
  )
}
