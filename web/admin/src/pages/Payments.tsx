import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
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
  { value: 'declined', label: 'Отклонён' },
  { value: 'cancelled', label: 'Отменён' },
]
const API_STATUS: Record<string, string> = { paid: 'confirmed', pending: 'pending', declined: 'declined', cancelled: 'cancelled' }
const EMPTY: Record<string, string> = {
  all: 'Платежей за период нет',
  paid: 'Оплаченных платежей за период нет',
  pending: 'Ожидающих платежей за период нет',
  declined: 'Отклонённых платежей за период нет',
  cancelled: 'Отменённых платежей за период нет',
}

interface Page {
  items: Payment[]
  total: number
  revenue: number
}

export default function Payments() {
  const navigate = useNavigate()
  // период всегда виден и уходит явно: сегодня и 29 дней назад (МСК), как default сервера
  // ?tab=pending (ссылка с Обзора) — очередь на проверке за всё время, как счётчик на Обзоре и в меню
  const tab = useSearchParams()[0].get('tab')
  const [f, setF] = useState(() =>
    tab && SEGMENTS.some((s) => s.value === tab)
      ? { segment: tab, from: tab === 'pending' ? '2000-01-01' : mskDate(new Date(), -29), to: mskDate(new Date()), page: 1 }
      : { segment: 'all', from: mskDate(new Date(), -29), to: mskDate(new Date()), page: 1 },
  )
  const [showFilters, setShowFilters] = useState(false)

  const params = new URLSearchParams({ status: API_STATUS[f.segment] ?? '', page: String(f.page) })
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
  const rows = result?.items ?? []
  const empty = !loaded ? 'Загрузка…' : !result ? 'Не удалось загрузить платежи' : EMPTY[f.segment]
  const offset = (f.page - 1) * PAGE_SIZE

  const now = new Date()
  const period = [f.from && `с ${formatDay(f.from, now)}`, f.to && `по ${formatDay(f.to, now)}`].filter(Boolean).join(' ') || 'Даты не выбраны: последние 30 дней'

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={f.segment} onChange={(segment) => filter({ segment })} />
        </div>
        <div className="grow" />
        <Button variant="gray" icon="filter" aria-expanded={showFilters} onClick={() => setShowFilters(!showFilters)}>
          Фильтры
        </Button>
      </div>
      {showFilters && (
        <Card className="admin-body">
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
            result
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
