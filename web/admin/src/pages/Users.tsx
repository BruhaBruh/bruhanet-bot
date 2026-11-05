import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { notWired } from '@/lib/not-wired'
import { Avatar, Button, DataTable, RowAction, SearchField, SegmentedControl, type DataTableColumn } from '@/ui'
import { get } from '../api'
import { formatCount, formatFullDay, formatRub } from '../format'
import { userLabel } from '../promo'
import type { UserSummary } from '../types'

const DEBOUNCE_MS = 300
const DASH = '—'

const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'active', label: 'Активные' },
  { value: 'trial', label: 'Пробный период' },
  { value: 'blocked', label: 'Заблокировали' },
]
// Статуса подписки в /api/users нет: сегменты кроме «Все» всегда пустые.
const EMPTY: Record<string, string> = {
  active: 'Активных пользователей нет',
  trial: 'Пользователей на пробном периоде нет',
  blocked: 'Заблокировавших бота нет',
}

const columns: DataTableColumn<UserSummary>[] = [
  {
    key: 'user',
    title: 'Пользователь',
    sortable: true,
    sortValue: userLabel,
    truncate: true,
    maxWidth: 240,
    render: (u) => (
      <span className="flex items-center gap-2.5">
        <Avatar name={userLabel(u)} id={u.telegramId} size={28} />
        <span className="truncate">{userLabel(u)}</span>
      </span>
    ),
  },
  { key: 'telegramId', title: 'Telegram ID', mono: true, width: 120 },
  { key: 'plan', title: 'Тариф', render: () => DASH },
  { key: 'confirmed', title: 'Платежей', numeric: true, sortable: true, width: 110, render: (u) => formatCount(u.confirmed) },
  { key: 'confirmedSum', title: 'Оплачено всего', numeric: true, sortable: true, width: 150, render: (u) => formatRub(u.confirmedSum) },
  { key: 'joined', title: 'С нами с', secondary: true, width: 140, render: (u) => formatFullDay(u.createdAt) },
  { key: 'status', title: 'Статус', width: 170, render: () => DASH },
]

export default function Users() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const urlQuery = params.get('q') ?? ''
  const query = urlQuery.trim()
  const [segment, setSegment] = useState('all')

  // Поле живёт своим стейтом, URL догоняет его с задержкой; ?q= извне (поиск в шапке, «назад») перезаписывает поле.
  const [text, setText] = useState(urlQuery)
  const [seenQuery, setSeenQuery] = useState(urlQuery)
  if (seenQuery !== urlQuery) {
    setSeenQuery(urlQuery)
    setText(urlQuery)
  }

  useEffect(() => {
    if (text === urlQuery) return
    const timer = setTimeout(() => setParams(text ? { q: text } : {}, { replace: true }), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [text, urlQuery, setParams])

  // Прежний список виден, пока грузится новый; items нет — ошибка загрузки.
  const [data, setData] = useState<{ items?: UserSummary[] }>()
  useEffect(() => {
    let stale = false
    get<{ items: UserSummary[] }>(`/api/users?q=${encodeURIComponent(query)}`).then(
      (r) => {
        if (!stale) setData({ items: r.items })
      },
      () => {
        if (!stale) setData({})
      },
    )
    return () => {
      stale = true
    }
  }, [query])

  const open = (u: UserSummary) => navigate(`/users/${u.telegramId}`)

  const isAll = segment === 'all'
  const rows = isAll ? (data?.items ?? []) : []
  const empty = !isAll ? EMPTY[segment] : !data ? 'Загрузка…' : !data.items ? 'Не удалось загрузить пользователей' : query ? 'Ничего не найдено' : 'Пользователей пока нет'

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={segment} onChange={setSegment} />
        </div>
        <div className="grow" />
        <SearchField
          className="w-full md:w-70"
          placeholder="ID или @username"
          aria-label="Поиск пользователей"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onClear={() => setText('')}
        />
        <Button variant="gray" icon="filter" onClick={notWired}>
          Фильтры
        </Button>
      </div>
      <div className="overflow-x-auto">
        <DataTable
          columns={columns}
          rows={rows}
          empty={empty}
          onRowClick={open}
          // У API нет страниц: весь ответ — одна страница.
          pagination={isAll && data?.items ? { page: 1, pages: 1, from: rows.length ? 1 : 0, to: rows.length, total: formatCount(rows.length) } : undefined}
          rowActions={(u) => <RowAction icon="more" label="Действия с пользователем" onClick={() => open(u)} />}
          mobileRow={(u) => ({
            before: <Avatar name={userLabel(u)} id={u.telegramId} size={40} />,
            title: <span className="block truncate">{userLabel(u)}</span>,
            subtitle: `с нами с ${formatFullDay(u.createdAt)}`,
            value: formatRub(u.confirmedSum),
            chevron: true,
          })}
        />
      </div>
    </>
  )
}
