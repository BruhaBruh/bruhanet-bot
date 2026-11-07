import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router'
import { Button, Card, DataTable, Dialog, Input, RowAction, RowMenu, SegmentedControl, StatusTag, toast, type DataTableColumn, type Tone } from '@/ui'
import { ApiError, get, post } from '../api'
import { formatCount, formatShortDate, plural } from '../format'
import { giftLink } from '../gift'
import { useMe } from '../me'
import { userLabel } from '../promo'
import { UserChip } from './user-chip'
import { DeleteDialog } from './delete-dialog'
import type { Gift, GiftStatus, Settings } from '../types'

const PAGE_SIZE = 50
const MAX_MONTHS = 120
const MAX_COUNT = 20

const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'pending', label: 'Ждут оплаты' },
  { value: 'ready', label: 'Активные' },
  { value: 'redeemed', label: 'Использованные' },
  { value: 'cancelled', label: 'Отозванные' },
]
const EMPTY: Record<string, string> = {
  all: 'Подарков ещё нет',
  pending: 'Подарков, ждущих оплаты, нет',
  ready: 'Активных подарков нет',
  redeemed: 'Использованных подарков нет',
  cancelled: 'Отозванных подарков нет',
}

const STATUS: Record<GiftStatus, [Tone, string]> = {
  pending: ['warning', 'Ждёт оплаты'],
  ready: ['success', 'Активен'],
  redeemed: ['neutral', 'Использован'],
  cancelled: ['destructive', 'Отозван'],
}

const statusTag = (g: Gift) => {
  const [tone, label] = STATUS[g.status]
  return <StatusTag tone={tone}>{label}</StatusTag>
}

const monthsLabel = (n: number) => `${n}\u00a0${plural(n, ['месяц', 'месяца', 'месяцев'])}`
const buyerLabel = (g: Gift) => userLabel({ name: g.buyerName, username: g.buyerUsername, telegramId: g.buyerId ?? 0 })
const source = (g: Gift) => (g.byAdmin ? 'выдан админом' : `куплен ${buyerLabel(g)}`)
const sourceCell = (g: Gift) => (g.byAdmin || !g.buyerId ? source(g) : <UserChip id={g.buyerId} label={buyerLabel(g)} prefix="куплен" />)
const redeemer = (g: Gift) => (g.redeemedBy ? userLabel({ name: g.redeemedName, username: g.redeemedUsername, telegramId: g.redeemedBy }) : '')
const shortCode = (g: Gift) => (g.code ? `${g.code.slice(0, 6)}…` : '—')

async function copyText(text: string, ok: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast(ok, { icon: 'check' })
  } catch {
    toast('Не удалось скопировать')
  }
}

/** Удалить можно готовый (отзыв и удаление одним шагом) и отозванный; ожидающий оплаты и использованный — история. */
const canDelete = (g: Gift) => g.status === 'ready' || g.status === 'cancelled'

const failure = (e: unknown, fallback: string) => (e instanceof ApiError && e.body.message) || fallback

/** «Создать подарок»: срок из тарифов или числом, сколько штук; после создания — ссылки с кнопками копирования. */
function CreateDialog({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { bot } = useMe()
  const [tariffs, setTariffs] = useState<number[]>([])
  const [months, setMonths] = useState('1')
  const [count, setCount] = useState('1')
  const [touched, setTouched] = useState(false)
  const [created, setCreated] = useState<Gift[]>()
  const sending = useRef(false)

  useEffect(() => {
    let stale = false
    get<Settings>('/api/settings').then(
      (s) => !stale && setTariffs(Object.keys(s.prices).map(Number).filter((n) => n >= 1 && n <= MAX_MONTHS).sort((a, b) => a - b)),
      () => {},
    )
    return () => {
      stale = true
    }
  }, [])

  const m = /^\d+$/.test(months) ? Number(months) : undefined
  const n = /^\d+$/.test(count) ? Number(count) : undefined
  const monthsOk = m !== undefined && m >= 1 && m <= MAX_MONTHS
  const countOk = n !== undefined && n >= 1 && n <= MAX_COUNT

  async function submit() {
    if (!monthsOk || !countOk) {
      setTouched(true)
      return
    }
    if (sending.current) return
    sending.current = true
    try {
      const r = await post<{ items: Gift[] }>('/api/gifts', { months: m, count: n })
      setCreated(r.items)
      onDone()
    } catch (e) {
      toast(failure(e, 'Не удалось создать подарок'))
    } finally {
      sending.current = false
    }
  }

  if (created) {
    const links = created.map((g) => giftLink(bot.username, g))
    return (
      <Dialog
        title="Подарки созданы"
        message={bot.username ? 'Каждая ссылка работает один раз.' : 'Имя бота неизвестно: скопируйте коды и соберите ссылку t.me/<бот>?start=gift_<код>.'}
        actions={[
          { label: 'Скопировать все', onClick: () => void copyText(created.map((g, i) => links[i] || g.code).join('\n'), 'Скопировано') },
          { label: 'Готово', style: 'cancel', onClick: onClose },
        ]}
      >
        <ul className="mt-3 flex max-h-60 flex-col gap-2 overflow-y-auto text-left">
          {created.map((g, i) => (
            <li key={g.id} className="flex items-center gap-2">
              <span className="min-w-0 grow break-all font-mono text-[13px]">{links[i] || g.code}</span>
              <RowAction icon="copy" label={`Скопировать ссылку ${i + 1}`} onClick={() => void copyText(links[i] || g.code, 'Ссылка скопирована')} />
            </li>
          ))}
        </ul>
      </Dialog>
    )
  }

  return (
    <Dialog
      title="Создать подарок"
      message="Готовые ссылки на подписку в подарок: получатель откроет её в боте."
      actions={[
        { label: 'Отмена', style: 'cancel', onClick: onClose },
        { label: 'Создать', onClick: () => void submit() },
      ]}
    >
      <div className="mt-3 flex flex-col items-center gap-3">
        {tariffs.length > 0 && (
          <div className="flex flex-wrap justify-center gap-2">
            {tariffs.map((t) => (
              <Button key={t} size="sm" variant={m === t ? 'tinted' : 'gray'} onClick={() => setMonths(String(t))}>
                {monthsLabel(t)}
              </Button>
            ))}
          </div>
        )}
        <Input
          label="Срок подарка"
          type="number"
          fit
          min={1}
          max={MAX_MONTHS}
          value={months}
          onChange={(e) => setMonths(e.target.value)}
          error={touched && !monthsOk ? `От 1 до ${MAX_MONTHS}` : undefined}
          after="мес."
        />
        <Input
          label="Сколько подарков"
          type="number"
          fit
          min={1}
          max={MAX_COUNT}
          value={count}
          onChange={(e) => setCount(e.target.value)}
          error={touched && !countOk ? `От 1 до ${MAX_COUNT}` : undefined}
          after="шт."
        />
      </div>
    </Dialog>
  )
}

export default function Gifts() {
  const { bot } = useMe()
  const [segment, setSegment] = useState('all')
  const [page, setPage] = useState(1)
  const [reloads, setReloads] = useState(0)
  const url = `/api/gifts?status=${segment}&page=${page}`
  // Ответ привязан к запросу: чужой не показывается, result нет — ошибка загрузки.
  const [data, setData] = useState<{ url: string; result?: { items: Gift[]; total: number } }>()
  useEffect(() => {
    let stale = false
    get<{ items: Gift[]; total: number }>(url).then(
      (result) => !stale && setData({ url, result }),
      () => !stale && setData({ url }),
    )
    return () => {
      stale = true
    }
  }, [url, reloads])

  const [creating, setCreating] = useState(false)
  const [cancelling, setCancelling] = useState<Gift>()
  const [deleting, setDeleting] = useState<Gift>()
  const [openId, setOpenId] = useState<number>()
  const busy = useRef(false)

  async function cancel(g: Gift) {
    setCancelling(undefined)
    if (busy.current) return
    busy.current = true
    try {
      await post(`/api/gifts/${g.id}/cancel`)
      toast('Подарок отозван', { icon: 'check' })
    } catch (e) {
      toast(failure(e, 'Не удалось отозвать подарок'))
    } finally {
      busy.current = false
      setReloads((n) => n + 1) // и после отказа: «уже использован» значит, что строка устарела
    }
  }

  function copyLink(g: Gift) {
    const link = giftLink(bot.username, g)
    if (!link) toast(g.code ? 'Имя бота неизвестно, ссылку не собрать' : 'Код появится после подтверждения платежа')
    else void copyText(link, 'Ссылка скопирована')
  }

  const columns: DataTableColumn<Gift>[] = [
    { key: 'code', title: 'Код', mono: true, strong: true, width: 130, render: shortCode },
    { key: 'months', title: 'Срок', numeric: true, width: 110, render: (g) => monthsLabel(g.months) },
    { key: 'source', title: 'Откуда', truncate: true, maxWidth: 220, render: sourceCell },
    { key: 'status', title: 'Статус', width: 150, render: statusTag },
    {
      key: 'redeemed',
      title: 'Получатель',
      truncate: true,
      maxWidth: 200,
      render: (g) =>
        g.redeemedBy ? (
          <Link to={`/users/${g.redeemedBy}`} className="focus-ring text-inherit" onClick={(e) => e.stopPropagation()}>
            <UserChip id={g.redeemedBy} label={redeemer(g)} />
          </Link>
        ) : (
          '—'
        ),
    },
    { key: 'date', title: 'Дата', secondary: true, width: 150, render: (g) => formatShortDate(g.redeemedAt ?? g.createdAt) },
  ]

  const loaded = data?.url === url ? data : undefined
  const result = loaded?.result
  const rows = result?.items ?? []
  const empty = !loaded ? 'Загрузка…' : !result ? 'Не удалось загрузить подарки' : EMPTY[segment]
  const offset = (page - 1) * PAGE_SIZE

  const expanded = (g: Gift) =>
    openId !== g.id ? null : (
      <div className="admin-table flex flex-col gap-2 bg-[color-mix(in_srgb,var(--page-bg)_55%,var(--surface))] px-5 py-3 whitespace-normal md:hidden">
        <span>Откуда: {sourceCell(g)}</span>
        {g.redeemedBy && (
          <span>
            Получатель: <UserChip id={g.redeemedBy} label={redeemer(g)} />
          </span>
        )}
        <div className="flex flex-wrap gap-2">
          {g.code && (
            <Button variant="gray" size="sm" icon="copy" onClick={() => copyLink(g)}>
              Скопировать ссылку
            </Button>
          )}
          {g.status === 'ready' && (
            <Button variant="gray" size="sm" onClick={() => setCancelling(g)}>
              Отозвать
            </Button>
          )}
          {canDelete(g) && (
            <Button variant="destructive" size="sm" onClick={() => setDeleting(g)}>
              Удалить
            </Button>
          )}
        </div>
      </div>
    )

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl
            options={SEGMENTS}
            value={segment}
            onChange={(v) => {
              setSegment(v)
              setPage(1)
            }}
          />
        </div>
        <div className="grow" />
        <Button icon="plus" onClick={() => setCreating(true)}>
          Создать подарок
        </Button>
      </div>
      {!bot.username && (
        <Card className="admin-body">
          <p className="footnote text-text-secondary">Имя бота неизвестно, поэтому ссылки не собираются. Код подарка виден в списке и в окне создания.</p>
        </Card>
      )}
      <div className="overflow-x-auto">
        <DataTable
          columns={columns}
          rows={rows}
          empty={empty}
          onRowClick={(g) => setOpenId(openId === g.id ? undefined : g.id)}
          expandedRow={expanded}
          rowActions={(g) => (
            <span className="inline-flex gap-0.5">
              {g.code && <RowAction icon="copy" label="Скопировать ссылку" onClick={() => copyLink(g)} />}
              {canDelete(g) && (
                <RowMenu
                  label="Действия с подарком"
                  items={[
                    ...(g.status === 'ready' ? [{ label: 'Отозвать', destructive: true, onSelect: () => setCancelling(g) }] : []),
                    { label: 'Удалить', destructive: true, onSelect: () => setDeleting(g) },
                  ]}
                />
              )}
            </span>
          )}
          mobileRow={(g) => ({
            title: <span className="font-mono font-semibold">{shortCode(g)}</span>,
            subtitle: [monthsLabel(g.months), source(g), formatShortDate(g.redeemedAt ?? g.createdAt)].join(' · '),
            after: statusTag(g),
            chevron: true,
          })}
          pagination={
            result
              ? {
                  page,
                  pages: Math.max(1, Math.ceil(result.total / PAGE_SIZE)),
                  from: rows.length ? offset + 1 : 0,
                  to: offset + rows.length,
                  total: formatCount(result.total),
                  onPrev: () => setPage(page - 1),
                  onNext: () => setPage(page + 1),
                }
              : undefined
          }
        />
      </div>
      {creating && <CreateDialog onClose={() => setCreating(false)} onDone={() => setReloads((n) => n + 1)} />}
      {cancelling && (
        <Dialog
          title="Отозвать подарок?"
          message={`Ссылка на ${monthsLabel(cancelling.months)} перестанет работать. Если подарок уже активирован, срок получателя не изменится.`}
          actions={[
            { label: 'Назад', style: 'cancel', onClick: () => setCancelling(undefined) },
            { label: 'Отозвать подарок', style: 'destructive', onClick: () => void cancel(cancelling) },
          ]}
        />
      )}
      {deleting && (
        <DeleteDialog
          title="Удалить подарок?"
          message={
            deleting.status === 'ready'
              ? 'Ссылка перестанет работать, подарок исчезнет из списка. Это нельзя отменить.'
              : 'Подарок исчезнет из списка. Это нельзя отменить.'
          }
          url={`/api/gifts/${deleting.id}`}
          done="Подарок удалён"
          conflict="Подарок ждёт оплаты или уже использован: его не удалить"
          onClose={() => setDeleting(undefined)}
          onDeleted={() => setReloads((n) => n + 1)}
          onStale={() => setReloads((n) => n + 1)}
        />
      )}
    </>
  )
}
