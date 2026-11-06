import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { Button, Cell, DataTable, StatCard } from '@/ui'
import { get } from '../api'
import { formatCount, formatDelta, formatRub, mskDate } from '../format'
import { useSyncKey } from '../layout/page-title'
import type { Payment, Promo } from '../types'
import { paymentColumns, paymentMobileRow } from './payment-columns'

const DASH = '—'
const RECENT_COUNT = 5
const panel = 'overflow-hidden rounded-lg bg-surface shadow-card'

interface PaymentsPage {
  items: Payment[]
  revenue: number
}

// undefined — грузится, null — не вышло: ошибка одной секции не трогает остальные.
function useGet<T>(path: string) {
  const syncKey = useSyncKey()
  const [data, setData] = useState<T | null>()
  useEffect(() => {
    let stale = false
    get<T>(path).then(
      (r) => !stale && setData(r),
      () => !stale && setData(null),
    )
    return () => {
      stale = true
    }
  }, [path, syncKey])
  return data
}

function revenuePaths() {
  const now = new Date()
  return {
    week: `/api/payments?from=${mskDate(now, -6)}&to=${mskDate(now)}`,
    prevWeek: `/api/payments?from=${mskDate(now, -13)}&to=${mskDate(now, -7)}`,
  }
}

export default function Overview() {
  const navigate = useNavigate()
  const [paths] = useState(revenuePaths)
  const users = useGet<{ total: number }>('/api/users')
  const promos = useGet<{ items: Promo[] }>('/api/promos?status=active')
  const week = useGet<PaymentsPage>(paths.week)
  const prevWeek = useGet<PaymentsPage>(paths.prevWeek)
  // Без from сервер отдаёт только 30 дней, а «последние» могут быть и старше.
  const recent = useGet<PaymentsPage>('/api/payments?from=2000-01-01')

  const revenueDelta = week && prevWeek ? formatDelta(week.revenue, prevWeek.revenue) : undefined

  return (
    <>
      <div className="grid grid-cols-[repeat(auto-fit,minmax(--spacing(55),1fr))] gap-3">
        <StatCard
          icon="users"
          iconColor="blue"
          label="Пользователи"
          value={users ? formatCount(users.total) : DASH}
          delta={users ? DASH : undefined}
          deltaTone="neutral"
          hint={users ? 'за 7 дней' : undefined}
        />
        <StatCard
          icon="card"
          iconColor="green"
          label="Выручка за 7 дней"
          value={week ? formatRub(week.revenue) : DASH}
          delta={revenueDelta}
          hint={revenueDelta ? 'к прошлой неделе' : undefined}
        />
        <StatCard
          icon="megaphone"
          iconColor="purple"
          label="Открываемость рассылок"
          value={DASH}
          delta={DASH}
          deltaTone="neutral"
          hint="последняя рассылка"
        />
        <StatCard
          icon="ticket"
          iconColor="orange"
          label="Активные промокоды"
          value={promos ? formatCount(promos.items.length) : DASH}
          delta={promos ? DASH : undefined}
          deltaTone="neutral"
          hint={promos ? 'применений за неделю' : undefined}
        />
      </div>

      <div className="flex flex-wrap items-start gap-6">
        <div className="min-w-0 grow-999 basis-130 overflow-x-auto">
          <DataTable
            title="Последние платежи"
            columns={paymentColumns}
            rows={recent?.items.slice(0, RECENT_COUNT) ?? []}
            mobileRow={paymentMobileRow}
            toolbar={
              <Button variant="plain" onClick={() => navigate('/payments')}>
                Все платежи
              </Button>
            }
            empty={
              recent === undefined ? 'Загрузка…' : recent === null ? 'Не удалось загрузить платежи' : 'Платежей пока нет'
            }
          />
        </div>
        <div className="flex min-w-70 grow basis-75 flex-col gap-3">
          <h2 className="headline px-1">Ближайшие рассылки</h2>
          <div className={panel}>
            <p className="admin-table px-4 py-6 text-center text-text-secondary">Запланированных рассылок нет</p>
          </div>
          <h2 className="headline mt-3 px-1">Быстрые действия</h2>
          <div className={panel}>
            <Cell title="Новая рассылка" accent href="/broadcasts/new" />
            <Cell title="Создать промокод" accent href="/promos/new" />
            <Cell title="Редактор сообщений" accent href="/texts" />
          </div>
        </div>
      </div>
    </>
  )
}
