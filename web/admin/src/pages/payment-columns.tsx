import { Link } from 'react-router'
import { Avatar, RowAction, StatusTag, toast, type CellProps, type DataTableColumn, type Tone } from '@/ui'
import { formatRub, formatShortDate } from '../format'
import { userLabel } from '../promo'
import type { Payment } from '../types'

type Column = DataTableColumn<Payment>

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

const planLabel = (p: Payment) => `${p.month}\u00a0мес.`

const idColumn: Column = { key: 'id', title: 'ID', mono: true, width: 120 }
const userColumn: Column = {
  key: 'user',
  title: 'Пользователь',
  maxWidth: 240,
  truncate: true,
  render: (p) => (
    // stopPropagation: клик по строке ведёт туда же, вторая запись в истории не нужна.
    <Link to={`/users/${p.telegramId}`} className="focus-ring flex items-center gap-2.5 text-inherit" onClick={(e) => e.stopPropagation()}>
      <Avatar name={userLabel(p)} id={p.telegramId} size={28} />
      <span className="flex min-w-0 flex-col">
        <span className="truncate">{userLabel(p)}</span>
        {p.username && <span className="caption-1 truncate text-text-secondary">{p.telegramId}</span>}
      </span>
    </Link>
  ),
}
// Способа оплаты нет в API: «Перевод» не выдумываем.
const methodColumn: Column = { key: 'method', title: 'Способ', secondary: true, render: () => '—' }
const priceColumn: Column = { key: 'price', title: 'Сумма', numeric: true, width: 120, render: (p) => formatRub(p.price) }
const statusColumn: Column = { key: 'status', title: 'Статус', width: 140, render: (p) => statusTag(p.status) }
const dateColumn: Column = { key: 'date', title: 'Дата', secondary: true, width: 150, render: (p) => formatShortDate(p.createdAt) }

export const paymentColumns: Column[] = [idColumn, userColumn, priceColumn, statusColumn, dateColumn]

export const paymentPageColumns: Column[] = [
  { ...idColumn, sortable: true, sortValue: (p) => Number(p.id) },
  { ...userColumn, sortable: true, sortValue: userLabel },
  { key: 'plan', title: 'Тариф', render: planLabel },
  methodColumn,
  { ...priceColumn, sortable: true },
  statusColumn,
  dateColumn,
]

// Платежи в карточке юзера: сам юзер уже в шапке.
export const userPayColumns: Column[] = [
  idColumn,
  { key: 'plan', title: 'Что оплачено', render: planLabel },
  methodColumn,
  { ...priceColumn, sortable: true },
  statusColumn,
  dateColumn,
]

export const paymentMobileRow = (p: Payment): CellProps => ({
  title: userLabel(p),
  subtitle: formatShortDate(p.createdAt),
  value: formatRub(p.price),
  after: statusTag(p.status),
})

export const paymentPageMobileRow = (p: Payment): CellProps => ({
  ...paymentMobileRow(p),
  subtitle: `${planLabel(p)} · ${formatShortDate(p.createdAt)}`,
  chevron: true,
})

export const userPayMobileRow = (p: Payment): CellProps => ({ ...paymentMobileRow(p), title: planLabel(p) })

async function copyId(id: Payment['id']) {
  try {
    await navigator.clipboard.writeText(String(id))
    toast(`ID ${id} скопирован`, { icon: 'check' })
  } catch {
    toast('Не удалось скопировать ID')
  }
}

export const payActions = (p: Payment) => <RowAction icon="copy" label="Скопировать ID платежа" onClick={() => void copyId(p.id)} />
