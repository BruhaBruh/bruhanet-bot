import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { Avatar, Button, Card, Checkbox, DataTable, Input, RowAction, SearchField, SegmentedControl, StatusTag, type DataTableColumn } from '@/ui'
import { get } from '../api'
import { formatCount, formatFullDay, formatRub, panelStatusLabels, panelStatusTones } from '../format'
import { userLabel } from '../promo'
import type { UserPage, UserSummary } from '../types'

const DEBOUNCE_MS = 300
const PAGE_SIZE = 50
const DASH = '—'

const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'active', label: 'Активные' },
  { value: 'trial', label: 'Пробный период' },
  { value: 'blocked_bot', label: 'Заблокировали' },
]
const EMPTY: Record<string, string> = {
  active: 'Активных пользователей нет',
  trial: 'Пользователей на пробном периоде нет',
  blocked_bot: 'Заблокировавших бота нет',
}

const displayName = (u: UserSummary) => u.name || userLabel(u)

function UserStatus({ user: u }: { user: UserSummary }) {
  if (u.blockedBotAt) {
    return (
      <StatusTag tone="destructive" dot>
        заблокировал бота
      </StatusTag>
    )
  }
  if (!u.panelStatus) return DASH
  return (
    <StatusTag tone={panelStatusTones[u.panelStatus] ?? 'neutral'} dot>
      {panelStatusLabels[u.panelStatus] ?? u.panelStatus}
    </StatusTag>
  )
}

const columns: DataTableColumn<UserSummary>[] = [
  {
    key: 'user',
    title: 'Пользователь',
    sortable: true,
    sortValue: displayName,
    truncate: true,
    maxWidth: 240,
    render: (u) => (
      <span className="flex items-center gap-2.5">
        <Avatar name={displayName(u)} id={u.telegramId} size={28} />
        <span className="flex min-w-0 flex-col">
          <span className="truncate">{displayName(u)}</span>
          {u.name && <span className="caption-1 truncate text-text-secondary">{userLabel(u)}</span>}
        </span>
      </span>
    ),
  },
  { key: 'telegramId', title: 'Telegram ID', mono: true, width: 120 },
  { key: 'plan', title: 'Тариф', render: (u) => (u.tariffMonths ? `${u.tariffMonths} мес.` : DASH) },
  { key: 'confirmed', title: 'Платежей', numeric: true, sortable: true, width: 110, render: (u) => formatCount(u.confirmed) },
  { key: 'confirmedSum', title: 'Оплачено всего', numeric: true, sortable: true, width: 150, render: (u) => formatRub(u.confirmedSum) },
  { key: 'joined', title: 'С нами с', secondary: true, width: 140, render: (u) => formatFullDay(u.createdAt) },
  { key: 'status', title: 'Статус', width: 170, render: (u) => <UserStatus user={u} /> },
]

export default function Users() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const urlQuery = params.get('q') ?? ''
  const query = urlQuery.trim()
  const [f, setF] = useState({ segment: 'all', hasPayments: false, hasUnpaidAdvances: false, from: '', to: '', page: 1 })
  const [showFilters, setShowFilters] = useState(false)

  // Поле живёт своим стейтом, URL догоняет его с задержкой; ?q= извне (поиск в шапке, «назад») перезаписывает поле.
  const [text, setText] = useState(urlQuery)
  const [seenQuery, setSeenQuery] = useState(urlQuery)
  if (seenQuery !== urlQuery) {
    setSeenQuery(urlQuery)
    setText(urlQuery)
    setF({ ...f, page: 1 })
  }

  useEffect(() => {
    if (text === urlQuery) return
    const timer = setTimeout(() => setParams(text ? { q: text } : {}, { replace: true }), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [text, urlQuery, setParams])

  const request = new URLSearchParams({ q: query })
  if (f.segment !== 'all') request.set('tab', f.segment)
  if (f.hasPayments) request.set('hasPayments', 'true')
  if (f.hasUnpaidAdvances) request.set('hasUnpaidAdvances', 'true')
  if (f.from) request.set('from', f.from)
  if (f.to) request.set('to', f.to)
  if (f.page > 1) request.set('page', String(f.page))
  const url = `/api/users?${request}`

  // Ответ привязан к запросу: чужой не показывается, result нет — ошибка загрузки.
  const [data, setData] = useState<{ url: string; result?: UserPage }>()
  useEffect(() => {
    let stale = false
    get<UserPage>(url).then(
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
  }, [url])

  // смена фильтра — снова с первой страницы
  const filter = (patch: Partial<typeof f>) => setF((f) => ({ ...f, ...patch, page: 1 }))
  const goTo = (page: number) => setF((f) => ({ ...f, page }))

  const open = (u: UserSummary) => navigate(`/users/${u.telegramId}`)

  const loaded = data?.url === url ? data : undefined
  const result = loaded?.result
  const rows = result?.items ?? []
  const filtered = query || f.hasPayments || f.hasUnpaidAdvances || f.from || f.to
  const empty = !loaded ? 'Загрузка…' : !result ? 'Не удалось загрузить пользователей' : filtered ? 'Ничего не найдено' : (EMPTY[f.segment] ?? 'Пользователей пока нет')
  const offset = (f.page - 1) * PAGE_SIZE

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={f.segment} onChange={(segment) => filter({ segment })} />
        </div>
        <div className="grow" />
        <SearchField
          className="w-full md:w-70"
          placeholder="Имя, ID или @username"
          aria-label="Поиск пользователей"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onClear={() => setText('')}
        />
        <Button variant="gray" icon="filter" aria-expanded={showFilters} onClick={() => setShowFilters(!showFilters)}>
          Фильтры
        </Button>
      </div>
      {showFilters && (
        <Card className="admin-body">
          <div className="flex flex-wrap gap-x-6 gap-y-3">
            <Checkbox label="Есть оплаты" checked={f.hasPayments} onChange={(hasPayments) => filter({ hasPayments })} />
            <Checkbox label="Есть неоплаченные авансы" checked={f.hasUnpaidAdvances} onChange={(hasUnpaidAdvances) => filter({ hasUnpaidAdvances })} />
          </div>
          <div className="flex flex-wrap gap-4">
            <Input type="date" label="Регистрация с" className="min-w-0 flex-1 basis-50" value={f.from} onChange={(e) => filter({ from: e.target.value })} />
            <Input type="date" label="Регистрация по" className="min-w-0 flex-1 basis-50" value={f.to} onChange={(e) => filter({ to: e.target.value })} />
          </div>
        </Card>
      )}
      <div className="overflow-x-auto">
        <DataTable
          columns={columns}
          rows={rows}
          empty={empty}
          onRowClick={open}
          pagination={
            result
              ? {
                  page: result.page,
                  pages: result.pages,
                  from: rows.length ? offset + 1 : 0,
                  to: offset + rows.length,
                  total: formatCount(result.total),
                  onPrev: () => goTo(f.page - 1),
                  onNext: () => goTo(f.page + 1),
                }
              : undefined
          }
          rowActions={(u) => <RowAction icon="more" label="Действия с пользователем" onClick={() => open(u)} />}
          mobileRow={(u) => ({
            before: <Avatar name={displayName(u)} id={u.telegramId} size={40} />,
            title: <span className="block truncate">{displayName(u)}</span>,
            subtitle: u.name ? `${userLabel(u)}, с нами с ${formatFullDay(u.createdAt)}` : `с нами с ${formatFullDay(u.createdAt)}`,
            value: formatRub(u.confirmedSum),
            chevron: true,
          })}
        />
      </div>
    </>
  )
}
