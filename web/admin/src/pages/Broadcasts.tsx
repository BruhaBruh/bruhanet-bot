import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { cn } from '@/lib/utils'
import { Button, Card, DataTable, Icon, Input, RowAction, RowMenu, SegmentedControl, toast, type DataTableColumn } from '@/ui'
import { get } from '../api'
import { audienceLabel } from '../compose/audience'
import { delivered, deleteMessage, statusTag, stats, titleOf, whenOf } from './broadcast-format'
import { formatCount } from '../format'
import { DeleteDialog } from './delete-dialog'
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

const columns: DataTableColumn<Broadcast>[] = [
  {
    key: 'title',
    title: 'Рассылка',
    render: (b) => {
      const hasPhoto = !!(b.photo || b.photoId)
      const label = hasPhoto ? 'С фото' : 'Только текст'
      return (
        <span className="flex items-center gap-2.5">
          <span
            title={label}
            className={cn(
              'inline-flex size-7 flex-none items-center justify-center rounded-xs',
              hasPhoto ? 'bg-accent-tint text-accent-strong' : 'bg-fill-tertiary text-text-secondary',
            )}
          >
            <Icon name={hasPhoto ? 'card' : 'message'} size={16} label={label} />
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
  { key: 'buttons', title: 'Кнопки', numeric: true, width: 90, render: (b) => (b.buttons.length ? String(b.buttons.length) : DASH) },
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
  // период по дате создания, МСК; пусто — без границы
  const [range, setRange] = useState({ from: '', to: '' })
  const [showFilters, setShowFilters] = useState(false)
  const [data, setData] = useState<Page>()
  const [loadError, setLoadError] = useState(false)
  // устаревший ответ (страница уже другая) не перетирает новый
  const seq = useRef(0)

  const load = useCallback(() => {
    const s = ++seq.current
    const status = STATUSES[segment]
    const period = `${range.from ? `&from=${range.from}` : ''}${range.to ? `&to=${range.to}` : ''}`
    return get<Page>(`/api/broadcasts?page=${page}${status ? `&status=${status}` : ''}${period}`).then(
      (r) => {
        if (s !== seq.current) return
        setData(r)
        setLoadError(false)
      },
      () => {
        if (s === seq.current) setLoadError(true)
      },
    )
  }, [page, segment, range])

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

  const filter = (patch: Partial<typeof range>) => {
    setData(undefined)
    setPage(1)
    setRange({ ...range, ...patch })
  }

  // черновик — сразу в редактор, остальные — на экран рассылки
  const open = (b: Broadcast) => navigate(b.status === 'draft' ? `/broadcasts/${b.id}/edit` : `/broadcasts/${b.id}`)

  const duplicate = (b: Broadcast) => {
    navigate('/broadcasts/new', {
      state: { text: b.text, photo: b.photo, photoId: b.photoId ?? undefined, audience: b.audience, audienceDays: b.audienceDays, silent: b.silent, buttons: b.buttons },
    })
    toast(`Копия «${titleOf(b)}» открыта в редакторе`, { icon: 'check' })
  }

  const [deleting, setDeleting] = useState<Broadcast>()
  // sending/sent — история со счётчиками, удалять нельзя
  const deletable = (b: Broadcast) => b.status === 'draft' || b.status === 'scheduled' || b.status === 'cancelled'
  const menu = (b: Broadcast) => deletable(b) && <RowMenu label="Действия с рассылкой" items={[{ label: 'Удалить', destructive: true, onSelect: () => setDeleting(b) }]} />

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
        <Button variant="gray" icon="filter" aria-expanded={showFilters} onClick={() => setShowFilters(!showFilters)}>
          Фильтры
        </Button>
        <Button icon="plus" onClick={() => navigate('/broadcasts/new')}>
          Новая рассылка
        </Button>
      </div>
      {showFilters && (
        <Card className="admin-body">
          <div className="flex flex-wrap gap-4">
            <Input type="date" label="С даты" className="min-w-0 flex-1 basis-50" max={range.to || undefined} value={range.from} onChange={(e) => filter({ from: e.target.value })} />
            <Input type="date" label="По дату" className="min-w-0 flex-1 basis-50" min={range.from || undefined} value={range.to} onChange={(e) => filter({ to: e.target.value })} />
          </div>
          <p className="footnote text-text-secondary">
            Список отфильтрован по дате создания рассылки (МСК).{' '}
            {(range.from || range.to) && (
              <Button variant="plain" size="sm" onClick={() => filter({ from: '', to: '' })}>
                Сбросить период
              </Button>
            )}
          </p>
        </Card>
      )}
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
              {menu(b)}
            </span>
          )}
          mobileRow={(b) => ({
            title: titleOf(b),
            // Счётчики — суть строки: переносятся, а не режутся.
            subtitle: <span className="block whitespace-normal">{`${whenOf(b)} · ${stats(b)}`}</span>,
            // Строка не кликается (onClick перекрывает onRowClick): внутри кнопки «Открыть»/«Изменить» и «Дублировать».
            onClick: undefined,
            after: (
              <>
                {statusTag(b)}
                <RowAction icon={b.status === 'draft' ? 'edit' : 'search'} label={b.status === 'draft' ? 'Изменить' : 'Открыть'} onClick={() => open(b)} />
                <RowAction icon="copy" label="Дублировать" onClick={() => duplicate(b)} />
                {menu(b)}
              </>
            ),
          })}
        />
      </div>
      {deleting && (
        <DeleteDialog
          title="Удалить рассылку?"
          message={deleteMessage(deleting)}
          url={`/api/broadcasts/${deleting.id}`}
          done="Рассылка удалена"
          conflict="Рассылка уже запущена, удалить нельзя"
          onClose={() => setDeleting(undefined)}
          onDeleted={() => void load()}
          onStale={() => void load()}
        />
      )}
    </>
  )
}
