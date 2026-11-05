import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { notWired } from '@/lib/not-wired'
import { cn } from '@/lib/utils'
import { Button, DataTable, Icon, RowAction, SegmentedControl, StatusTag, toast, type DataTableColumn } from '@/ui'
import { get } from '../api'
import { visibleText } from '../editor/serialize'
import { formatCount, formatShortDate } from '../format'
import type { Broadcast } from '../types'

const DASH = '—'
const PAGE_SIZE = 20 // broadcastsPage в api
const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'scheduled', label: 'Запланированные' },
  { value: 'sent', label: 'Отправленные' },
  { value: 'draft', label: 'Черновики' },
]
const EMPTY: Record<string, string> = {
  all: 'Рассылок ещё не было',
  scheduled: 'Запланированных рассылок нет',
  sent: 'Отправленных рассылок нет',
  draft: 'Черновиков нет',
}

const titleOf = (b: Broadcast) =>
  visibleText(b.text)
    .split('\n')
    .find((line) => line.trim())
    ?.trim() ?? 'Без названия'

const stats = (b: Broadcast) => `доставлено ${formatCount(b.sent)} из ${formatCount(b.total)}, не доставлено ${formatCount(b.failed)}`

const statusTag = (b: Broadcast) =>
  b.finishedAt ? <StatusTag tone="success">Отправлено</StatusTag> : <StatusTag tone="accent">Отправляется</StatusTag>

const columns: DataTableColumn<Broadcast>[] = [
  {
    key: 'title',
    title: 'Рассылка',
    render: (b) => {
      const label = b.photo ? 'С фото' : 'Только текст'
      return (
        <span className="flex items-center gap-2.5">
          <span
            title={label}
            className={cn(
              'inline-flex size-7 flex-none items-center justify-center rounded-xs',
              b.photo ? 'bg-accent-tint text-accent-strong' : 'bg-fill-tertiary text-text-secondary',
            )}
          >
            <Icon name={b.photo ? 'card' : 'message'} size={16} label={label} />
          </span>
          <span className="flex max-w-70 min-w-0 flex-col whitespace-nowrap">
            <span className="truncate font-medium">{titleOf(b)}</span>
            <span className="caption-1 truncate text-text-secondary">{stats(b)}</span>
          </span>
        </span>
      )
    },
  },
  { key: 'audience', title: 'Аудитория', secondary: true, truncate: true, maxWidth: 220, render: () => 'Все пользователи' },
  { key: 'total', title: 'Получатели', numeric: true, sortable: true, width: 120, render: (b) => formatCount(b.total) },
  { key: 'opens', title: 'Открыли', numeric: true, sortable: true, width: 100, render: () => DASH },
  { key: 'buttons', title: 'Кнопки', numeric: true, width: 90, render: () => DASH },
  { key: 'date', title: 'Отправка', secondary: true, width: 140, render: (b) => formatShortDate(b.createdAt) },
  { key: 'status', title: 'Статус', width: 160, render: statusTag },
]

interface Page {
  items: Broadcast[]
  total: number
}

export default function Broadcasts() {
  const navigate = useNavigate()
  const [segment, setSegment] = useState('all')
  const [page, setPage] = useState(1)
  const [data, setData] = useState<Page>()
  const [loadError, setLoadError] = useState(false)
  // устаревший ответ (страница уже другая) не перетирает новый
  const seq = useRef(0)

  const load = useCallback(() => {
    const s = ++seq.current
    return get<Page>(`/api/broadcasts?page=${page}`).then(
      (r) => {
        if (s !== seq.current) return
        setData(r)
        setLoadError(false)
      },
      () => {
        if (s === seq.current) setLoadError(true)
      },
    )
  }, [page])

  useEffect(() => {
    void load()
  }, [load])

  // автообновление, пока есть незавершённые и вкладка видна
  const sending = !!data?.items.some((b) => !b.finishedAt)
  useEffect(() => {
    if (!sending) return
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') void load()
    }, 5000)
    return () => clearInterval(timer)
  }, [sending, load])

  const goTo = (next: number) => {
    setData(undefined)
    setPage(next)
  }

  const duplicate = (b: Broadcast) => {
    navigate('/broadcasts/new', { state: { text: b.text, photo: b.photo } })
    toast(`Копия «${titleOf(b)}» открыта в редакторе`, { icon: 'check' })
  }

  // Отложенных рассылок и черновиков в API нет: эти сегменты всегда пустые.
  const hasData = segment === 'all' || segment === 'sent'
  const items = data?.items ?? []
  // ponytail: «Отправленные» фильтруются на клиенте в пределах страницы, фильтра статуса в API нет.
  const rows = !hasData ? [] : segment === 'sent' ? items.filter((b) => b.finishedAt) : items
  const total = data?.total ?? 0
  const from = (page - 1) * PAGE_SIZE

  const empty = !hasData
    ? EMPTY[segment]
    : loadError
      ? 'Не удалось загрузить рассылки'
      : data
        ? EMPTY[segment]
        : 'Загрузка…'

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={segment} onChange={setSegment} />
        </div>
        <div className="grow" />
        <Button variant="gray" icon="filter" onClick={notWired}>
          Фильтры
        </Button>
        <Button icon="plus" onClick={() => navigate('/broadcasts/new')}>
          Новая рассылка
        </Button>
      </div>
      <div className="overflow-x-auto">
        <DataTable
          columns={columns}
          rows={rows}
          empty={empty}
          onRowClick={notWired}
          pagination={
            hasData && data
              ? {
                  page,
                  pages: Math.max(1, Math.ceil(total / PAGE_SIZE)),
                  from: items.length ? from + 1 : 0,
                  to: from + items.length,
                  total: formatCount(total),
                  onPrev: () => goTo(page - 1),
                  onNext: () => goTo(page + 1),
                }
              : undefined
          }
          rowActions={(b) => (
            <span className="inline-flex gap-0.5">
              <RowAction icon="copy" label="Дублировать" onClick={() => duplicate(b)} />
              <RowAction icon="more" label="Действия с рассылкой" onClick={notWired} />
            </span>
          )}
          mobileRow={(b) => ({
            title: titleOf(b),
            subtitle: `${formatShortDate(b.createdAt)} · ${stats(b)}`,
            // Строка не кликается (onClick перекрывает onRowClick): в ней кнопка «Дублировать».
            onClick: undefined,
            after: (
              <>
                {statusTag(b)}
                <RowAction icon="copy" label="Дублировать" onClick={() => duplicate(b)} />
              </>
            ),
          })}
        />
      </div>
    </>
  )
}
