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

const isDevices = (p: Payment) => p.kind === 'devices'
// Подписка: «3 мес.» и «+2 устр.», если в цену входят доп. устройства; докупка: «+2 устр.» без срока.
const planLabel = (p: Payment) => {
  const extra = p.extraDevices ? `+${p.extraDevices}\u00a0устр.` : ''
  if (isDevices(p)) return extra
  return [`${p.month}\u00a0мес.`, extra].filter(Boolean).join(' · ')
}
const planCell = (p: Payment) => (
  <span className="inline-flex flex-wrap items-center gap-1.5">
    <StatusTag tone={isDevices(p) ? 'accent' : 'neutral'}>{isDevices(p) ? 'Устройства' : 'Подписка'}</StatusTag>
    <span>{planLabel(p)}</span>
  </span>
)

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
// «CODE · было M ₽»; у платежа без промокода пусто.
const promoNote = (p: Payment) => [p.promoCode, p.basePrice != null && `было ${formatRub(p.basePrice)}`].filter(Boolean).join(' · ')
// У оплаченного важна дата подтверждения: по ней считается выручка.
const paymentDate = (p: Payment) => (p.status === 'confirmed' && p.resolvedAt) || p.createdAt

const priceColumn: Column = {
  key: 'price',
  title: 'Сумма',
  numeric: true,
  width: 120,
  render: (p) => {
    const note = promoNote(p)
    return (
      <span className="flex flex-col items-end">
        <span>{formatRub(p.price)}</span>
        {note && <span className="caption-1 font-normal text-text-secondary">{note}</span>}
      </span>
    )
  },
}
const statusColumn: Column = {
  key: 'status',
  title: 'Статус',
  width: 140,
  render: (p) => (
    <span className="inline-flex items-center gap-1.5">
      {statusTag(p.status)}
      {p.advanceGranted && <StatusTag>аванс</StatusTag>}
    </span>
  ),
}
const dateColumn: Column = { key: 'date', title: 'Дата', secondary: true, width: 150, render: (p) => formatShortDate(paymentDate(p)) }

export const paymentColumns: Column[] = [idColumn, userColumn, priceColumn, statusColumn, dateColumn]

export const paymentPageColumns: Column[] = [
  { ...idColumn, sortable: true, sortValue: (p) => Number(p.id) },
  { ...userColumn, sortable: true, sortValue: userLabel },
  { key: 'plan', title: 'Тариф', render: planCell },
  { ...priceColumn, sortable: true },
  statusColumn,
  dateColumn,
]

// Платежи в карточке юзера: сам юзер уже в шапке.
export const userPayColumns: Column[] = [
  idColumn,
  { key: 'plan', title: 'Что оплачено', render: planCell },
  { ...priceColumn, sortable: true },
  statusColumn,
  dateColumn,
]

const mobileSubtitle = (p: Payment, ...lead: string[]) =>
  [...lead, formatShortDate(paymentDate(p)), p.advanceGranted && 'аванс', promoNote(p)].filter(Boolean).join(' · ')

export const paymentMobileRow = (p: Payment): CellProps => ({
  title: userLabel(p),
  subtitle: mobileSubtitle(p),
  value: formatRub(p.price),
  after: statusTag(p.status),
})

export const paymentPageMobileRow = (p: Payment): CellProps => ({
  ...paymentMobileRow(p),
  subtitle: mobileSubtitle(p, isDevices(p) ? `Устройства ${planLabel(p)}` : planLabel(p)),
  chevron: true,
})

export const userPayMobileRow = (p: Payment): CellProps => ({ ...paymentMobileRow(p), title: isDevices(p) ? `Устройства ${planLabel(p)}` : planLabel(p) })

async function copyId(id: Payment['id']) {
  try {
    await navigator.clipboard.writeText(String(id))
    toast(`ID ${id} скопирован`, { icon: 'check' })
  } catch {
    toast('Не удалось скопировать ID')
  }
}

export const payActions = (p: Payment) => <RowAction icon="copy" label="Скопировать ID платежа" onClick={() => void copyId(p.id)} />
