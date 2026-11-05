import { Avatar, StatusTag, type CellProps, type DataTableColumn, type Tone } from '@/ui'
import { formatRub, formatShortDate } from '../format'
import type { Payment } from '../types'

const STATUS: Record<string, [Tone, string]> = {
  confirmed: ['success', 'Оплачено'],
  pending: ['warning', 'Ожидает'],
  declined: ['destructive', 'Отклонён'],
  cancelled: ['neutral', 'Отменён'],
}

export function statusTag(status: string) {
  const [tone, label] = STATUS[status] ?? ['neutral', status]
  return (
    <StatusTag tone={tone} dot>
      {label}
    </StatusTag>
  )
}

const userLabel = (p: Payment) => (p.username ? `@${p.username}` : String(p.telegramId))

export const paymentColumns: DataTableColumn<Payment>[] = [
  { key: 'id', title: 'ID', mono: true, width: 120 },
  {
    key: 'user',
    title: 'Пользователь',
    maxWidth: 240,
    truncate: true,
    render: (p) => (
      <span className="flex items-center gap-2.5">
        <Avatar name={userLabel(p)} id={p.telegramId} size={28} />
        <span className="flex min-w-0 flex-col">
          <span className="truncate">{userLabel(p)}</span>
          {p.username && <span className="caption-1 truncate text-text-secondary">{p.telegramId}</span>}
        </span>
      </span>
    ),
  },
  { key: 'price', title: 'Сумма', numeric: true, width: 120, render: (p) => formatRub(p.price) },
  { key: 'status', title: 'Статус', width: 140, render: (p) => statusTag(p.status) },
  { key: 'date', title: 'Дата', secondary: true, width: 150, render: (p) => formatShortDate(p.createdAt) },
]

export const paymentMobileRow = (p: Payment): CellProps => ({
  title: userLabel(p),
  subtitle: formatShortDate(p.createdAt),
  value: formatRub(p.price),
  after: statusTag(p.status),
})
