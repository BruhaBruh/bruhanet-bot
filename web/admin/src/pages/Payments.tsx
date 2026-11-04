import { Cell, Info, Input, Pagination, Section, Select, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useEffect, useState } from 'react'
import { get } from '../api'
import { formatRub, mskDate, statusLabels } from '../format'
import type { Payment } from '../types'
import { PaymentsTable } from './PaymentsTable'

const pageSize = 50

export default function Payments() {
  // период всегда виден и уходит явно: сегодня и 29 дней назад (МСК), как default сервера
  const [f, setF] = useState(() => ({ status: '', from: mskDate(new Date(), -29), to: mskDate(new Date()), page: 1 }))
  const [data, setData] = useState({ items: [] as Payment[], total: 0, revenue: 0 })
  const [error, setError] = useState(false)

  useEffect(() => {
    let stale = false
    const p = new URLSearchParams({ status: f.status, page: String(f.page) })
    if (f.from) p.set('from', f.from)
    if (f.to) p.set('to', f.to)
    get<{ items: Payment[]; total: number; revenue: number }>(`/api/payments?${p}`).then(
      (r) => {
        if (stale) return
        setData(r)
        setError(false)
      },
      () => !stale && setError(true),
    )
    return () => {
      stale = true
    }
  }, [f])

  // смена фильтра — снова с первой страницы
  const filter = (patch: Partial<typeof f>) => setF((f) => ({ ...f, ...patch, page: 1 }))
  const pages = Math.max(1, Math.ceil(data.total / pageSize))

  return (
    <>
      <VisuallyHidden Component="h1">Платежи</VisuallyHidden>
      <Section header="Фильтры">
        <Select aria-label="Статус" value={f.status} onChange={(e) => filter({ status: e.target.value })}>
          <option value="">все статусы</option>
          {Object.entries(statusLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
        <Input type="date" aria-label="С даты" value={f.from} onChange={(e) => filter({ from: e.target.value })} />
        <Input type="date" aria-label="По дату" value={f.to} onChange={(e) => filter({ to: e.target.value })} />
      </Section>
      {error && (
        <p className="tg-err pad" role="alert">
          Не удалось загрузить
        </p>
      )}
      <Section header="Выручка (по дате подтверждения за период)">
        <Cell
          after={
            <Info type="text" subtitle={`всего платежей: ${data.total}`}>
              {formatRub(data.revenue)}
            </Info>
          }
        >
          Выручка
        </Cell>
      </Section>
      <PaymentsTable header="Платежи" items={data.items} showUser />
      <p className="hint">Список отфильтрован по дате создания платежа.</p>
      {pages > 1 && <Pagination className="pager" count={pages} page={f.page} hidePrevButton hideNextButton onChange={(_, page) => setF((f) => ({ ...f, page }))} />}
    </>
  )
}
