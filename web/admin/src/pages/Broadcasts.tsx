import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { notWired } from '@/lib/not-wired'
import { cn } from '@/lib/utils'
import { Button, DataTable, Icon, RowAction, SegmentedControl, StatusTag, toast, type DataTableColumn, type Tone } from '@/ui'
import { get } from '../api'
import { audienceLabel } from '../compose/audience'
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
// вкладка → фильтр статусов на сервере; «Отправленные» включают идущие прямо сейчас
const STATUSES: Record<string, string> = { all: '', scheduled: 'scheduled', sent: 'sending,sent', draft: 'draft' }
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

const stats = (b: Broadcast) => {
  switch (b.status) {
    case 'draft':
      return 'черновик, не отправлялась'
    case 'scheduled':
      return b.scheduledAt ? `уйдёт ${formatShortDate(b.scheduledAt)} (МСК)` : 'запланирована'
    case 'cancelled':
      return 'отменена, не отправлялась'
    default:
      return `доставлено ${formatCount(b.sent)} из ${formatCount(b.total)}, не доставлено ${formatCount(b.failed)}`
  }
}

const TAGS: Record<Broadcast['status'], { tone: Tone; label: string }> = {
  draft: { tone: 'neutral', label: 'Черновик' },
  scheduled: { tone: 'warning', label: 'Запланирована' },
  sending: { tone: 'accent', label: 'Отправляется' },
  sent: { tone: 'success', label: 'Отправлена' },
  cancelled: { tone: 'neutral', label: 'Отменена' },
}
const statusTag = (b: Broadcast) => <StatusTag tone={TAGS[b.status].tone}>{TAGS[b.status].label}</StatusTag>

// Когда ушла или уйдёт; у черновика — когда менялся
const whenOf = (b: Broadcast) => formatShortDate(b.status === 'scheduled' && b.scheduledAt ? b.scheduledAt : b.status === 'draft' ? b.updatedAt : b.createdAt)

const delivered = (b: Broadcast) => b.status === 'sending' || b.status === 'sent'

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
  { key: 'audience', title: 'Аудитория', secondary: true, truncate: true, maxWidth: 220, render: (b) => audienceLabel(b.audience, String(b.audienceDays ?? '')) },
  { key: 'total', title: 'Получатели', numeric: true, sortable: true, width: 120, render: (b) => (delivered(b) ? formatCount(b.total) : DASH) },
  { key: 'opens', title: 'Открыли', numeric: true, sortable: true, width: 100, render: () => DASH },
  { key: 'buttons', title: 'Кнопки', numeric: true, width: 90, render: () => DASH },
  { key: 'date', title: 'Отправка', secondary: true, width: 140, render: whenOf },
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
    const status = STATUSES[segment]
    return get<Page>(`/api/broadcasts?page=${page}${status ? `&status=${status}` : ''}`).then(
      (r) => {
        if (s !== seq.current) return
        setData(r)
        setLoadError(false)
      },
      () => {
        if (s === seq.current) setLoadError(true)
      },
    )
  }, [page, segment])

  useEffect(() => {
    void load()
  }, [load])

  // автообновление, пока есть незавершённые и вкладка видна
  const sending = !!data?.items.some((b) => b.status === 'sending')
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
  const pick = (next: string) => {
    if (next === segment) return
    setData(undefined)
    setPage(1)
    setSegment(next)
  }

  const open = (b: Broadcast) => {
    if (b.status === 'draft' || b.status === 'scheduled') navigate(`/broadcasts/${b.id}/edit`)
    else notWired()
  }

  const duplicate = (b: Broadcast) => {
    navigate('/broadcasts/new', {
      state: { text: b.text, photo: b.photo, audience: b.audience, audienceDays: b.audienceDays, silent: b.silent, buttons: b.buttons },
    })
    toast(`Копия «${titleOf(b)}» открыта в редакторе`, { icon: 'check' })
  }

  const rows = data?.items ?? []
  const total = data?.total ?? 0
  const from = (page - 1) * PAGE_SIZE

  const empty = loadError ? 'Не удалось загрузить рассылки' : data ? EMPTY[segment] : 'Загрузка…'

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={segment} onChange={pick} />
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
          onRowClick={open}
          pagination={
            data
              ? {
                  page,
                  pages: Math.max(1, Math.ceil(total / PAGE_SIZE)),
                  from: rows.length ? from + 1 : 0,
                  to: from + rows.length,
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
            // Счётчики — суть строки: переносятся, а не режутся.
            subtitle: <span className="block whitespace-normal">{`${whenOf(b)} · ${stats(b)}`}</span>,
            // Строка не кликается (onClick перекрывает onRowClick): внутри кнопки «Изменить»/«Дублировать».
            onClick: undefined,
            after: (
              <>
                {statusTag(b)}
                {(b.status === 'draft' || b.status === 'scheduled') && <RowAction icon="edit" label="Изменить" onClick={() => open(b)} />}
                <RowAction icon="copy" label="Дублировать" onClick={() => duplicate(b)} />
              </>
            ),
          })}
        />
      </div>
    </>
  )
}
