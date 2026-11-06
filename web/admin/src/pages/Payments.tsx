import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Button, Card, DataTable, Input, SegmentedControl, StatCard } from '@/ui'
import { get } from '../api'
import { formatCount, formatDay, formatRub, mskDate } from '../format'
import type { Payment } from '../types'
import { paymentPageColumns, paymentPageMobileRow } from './payment-columns'
import { usePaymentReview } from './payment-review'

const PAGE_SIZE = 50

const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'paid', label: 'Оплачено' },
  { value: 'pending', label: 'Ожидают' },
  { value: 'refund', label: 'Возвраты' },
]
const API_STATUS: Record<string, string> = { paid: 'confirmed', pending: 'pending' }
const STATUS_OPTIONS = [
  { value: 'any', label: 'Любой' },
  { value: 'confirmed', label: 'Оплачено' },
  { value: 'pending', label: 'Ожидает' },
  { value: 'declined', label: 'Отклонён' },
  { value: 'cancelled', label: 'Отменён' },
]
const EMPTY: Record<string, string> = {
  all: 'Платежей за период нет',
  paid: 'Оплаченных платежей за период нет',
  pending: 'Ожидающих платежей за период нет',
  refund: 'Возвратов нет',
}

interface Page {
  items: Payment[]
  total: number
  revenue: number
}

export default function Payments() {
  const navigate = useNavigate()
  // период всегда виден и уходит явно: сегодня и 29 дней назад (МСК), как default сервера
  const [f, setF] = useState(() => ({ segment: 'all', status: 'any', from: mskDate(new Date(), -29), to: mskDate(new Date()), page: 1 }))
  const [showFilters, setShowFilters] = useState(false)

  // Статус из фильтров важнее вкладки; смена вкладки его сбрасывает.
  const overridden = f.status !== 'any'
  const params = new URLSearchParams({ status: overridden ? f.status : (API_STATUS[f.segment] ?? ''), page: String(f.page) })
  if (f.from) params.set('from', f.from)
  if (f.to) params.set('to', f.to)
  const url = `/api/payments?${params}`

  // Ответ привязан к запросу: чужой не показывается, result нет — ошибка загрузки.
  const [data, setData] = useState<{ url: string; result?: Page }>()
  const [reloads, setReloads] = useState(0)
  const review = usePaymentReview(
    () => setReloads((n) => n + 1),
    (p) => navigate(`/users/${p.telegramId}`),
  )
  useEffect(() => {
    let stale = false
    get<Page>(url).then(
      (result) => {
        if (!stale) setData({ url, result })
      },
      () => {
        if (!stale) setData({ url })
      },
    )
    return () => {
      stale = true
    }
  }, [url, reloads])

  // смена фильтра — снова с первой страницы
  const filter = (patch: Partial<typeof f>) => setF((f) => ({ ...f, ...patch, page: 1 }))
  const goTo = (page: number) => setF((f) => ({ ...f, page }))

  const loaded = data?.url === url ? data : undefined
  const result = loaded?.result
  // Возвратов нет в API: сегмент всегда пустой.
  const isRefund = !overridden && f.segment === 'refund'
  const rows = isRefund ? [] : (result?.items ?? [])
  const empty = isRefund ? EMPTY.refund : !loaded ? 'Загрузка…' : !result ? 'Не удалось загрузить платежи' : overridden ? 'Платежей с таким статусом за период нет' : EMPTY[f.segment]
  const offset = (f.page - 1) * PAGE_SIZE

  const now = new Date()
  const period = [f.from && `с ${formatDay(f.from, now)}`, f.to && `по ${formatDay(f.to, now)}`].filter(Boolean).join(' ') || 'Даты не выбраны: последние 30 дней'

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={overridden ? undefined : f.segment} onChange={(segment) => filter({ segment, status: 'any' })} />
        </div>
        <div className="grow" />
        <Button variant="gray" icon="filter" aria-expanded={showFilters} onClick={() => setShowFilters(!showFilters)}>
          Фильтры
        </Button>
      </div>
      {showFilters && (
        <Card className="admin-body">
          <div role="group" aria-label="Статус" className="flex flex-col gap-2">
            <span className="admin-label px-0.5 text-text-secondary">Статус</span>
            <SegmentedControl className="self-start" options={STATUS_OPTIONS} value={f.status} onChange={(status) => filter({ status })} />
          </div>
          <div className="flex flex-wrap gap-4">
            <Input type="date" label="С даты" className="min-w-0 flex-1 basis-50" value={f.from} onChange={(e) => filter({ from: e.target.value })} />
            <Input type="date" label="По дату" className="min-w-0 flex-1 basis-50" value={f.to} onChange={(e) => filter({ to: e.target.value })} />
          </div>
          <p className="footnote text-text-secondary">Список отфильтрован по дате создания платежа.</p>
        </Card>
      )}
      <StatCard className="self-start" label="Выручка по дате подтверждения за период" value={result ? formatRub(result.revenue) : '—'} hint={period} />
      <div className="overflow-x-auto">
        <DataTable
          columns={paymentPageColumns}
          rows={rows}
          empty={empty}
          onRowClick={(p) => navigate(`/users/${p.telegramId}`)}
          rowActions={review.rowActions}
          expandedRow={review.expandedRow}
          mobileRow={(p) => ({ ...paymentPageMobileRow(p), onClick: () => review.toggle(p) })}
          pagination={
            result && !isRefund
              ? {
                  page: f.page,
                  pages: Math.max(1, Math.ceil(result.total / PAGE_SIZE)),
                  from: rows.length ? offset + 1 : 0,
                  to: offset + rows.length,
                  total: formatCount(result.total),
                  onPrev: () => goTo(f.page - 1),
                  onNext: () => goTo(f.page + 1),
                }
              : undefined
          }
        />
      </div>
      {review.dialogs}
    </>
  )
}
