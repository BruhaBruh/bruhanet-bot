import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { Button, Card, DataTable, Dialog, RowAction, RowMenu, SegmentedControl, Spinner, StatusTag, toast, type DataTableColumn, type Tone } from '@/ui'
import { get, post } from '../api'
import { formatCount, formatDay, formatShortDate, plural } from '../format'
import { DeleteDialog } from './delete-dialog'
import { kindLabels, promoStatus, promoUntil, promoValue, promosQuery, userLabel, type PromoStatus } from '../promo'
import type { Promo, PromoActivation } from '../types'

const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'active', label: 'Активные' },
  { value: 'expired', label: 'Истёкшие' },
  { value: 'deleted', label: 'Удалённые' },
]
const EMPTY: Record<string, string> = {
  all: 'Промокодов ещё нет',
  active: 'Активных промокодов нет',
  expired: 'Истёкших промокодов нет',
  deleted: 'Удалённых промокодов нет',
}

const STATUS: Record<PromoStatus, [Tone, string]> = {
  active: ['success', 'Активен'],
  upcoming: ['accent', 'Ещё не действует'],
  expired: ['neutral', 'Истёк'],
  limit: ['warning', 'Лимит исчерпан'],
  disabled: ['neutral', 'Выключен'],
  deleted: ['neutral', 'Удалён'],
}

const statusTag = (p: Promo) => {
  const [tone, label] = STATUS[promoStatus(p)]
  return <StatusTag tone={tone}>{label}</StatusTag>
}

/** Текст подтверждения: с историей код скрывается, без неё стирается без следа. */
const deleteMessage = (p: Promo) =>
  p.used > 0
    ? `Использован ${p.used}\u00a0${plural(p.used, ['раз', 'раза', 'раз'])}: код ${p.code} перестанет действовать и скроется из списка, история сохранится.`
    : `Код ${p.code} будет удалён без следа, потом его можно создать снова.`

const KIND_FILTER = [{ value: 'any', label: 'Любой' }, ...(['percent', 'rub', 'days'] as const).map((value) => ({ value, label: kindLabels[value] }))]
const TARGET_FILTER = [
  { value: 'any', label: 'Любые' },
  { value: 'all', label: 'Для всех' },
  { value: 'new', label: 'Для новых' },
  { value: 'user', label: 'Конкретному пользователю' },
]

const target = (p: Promo) =>
  p.telegramId ? userLabel({ username: p.username, telegramId: p.telegramId }) : p.onlyNew ? 'Новые пользователи' : 'Все'

const limitOf = (p: Promo) => (p.maxActivations == null ? '∞' : formatCount(p.maxActivations))

const columns: DataTableColumn<Promo>[] = [
  { key: 'code', title: 'Код', mono: true, strong: true, sortable: true, truncate: true, maxWidth: 240 },
  { key: 'kind', title: 'Тип', secondary: true, render: (p) => kindLabels[p.kind] },
  { key: 'value', title: 'Значение', numeric: true, width: 110, render: promoValue },
  { key: 'target', title: 'Для кого', truncate: true, maxWidth: 200, render: target },
  {
    key: 'used',
    title: 'Активации',
    numeric: true,
    sortable: true,
    width: 130,
    render: (p) => (
      <span>
        {formatCount(p.used)}
        <span className="text-text-secondary"> / {limitOf(p)}</span>
      </span>
    ),
  },
  { key: 'until', title: 'Действует до', secondary: true, width: 130, render: (p) => (p.validUntil ? formatDay(p.validUntil, new Date()) : 'бессрочно') },
  { key: 'status', title: 'Статус', width: 150, render: statusTag },
]

async function copyCode(code: string) {
  try {
    await navigator.clipboard.writeText(code)
    toast(`Код ${code} скопирован`, { icon: 'check' })
  } catch {
    toast('Не удалось скопировать код')
  }
}

export default function Promos() {
  const navigate = useNavigate()
  const [segment, setSegment] = useState('all')
  const [showFilters, setShowFilters] = useState(false)
  const [kind, setKind] = useState('')
  const [who, setWho] = useState('')
  const apiStatus = promosQuery(segment, kind, who)
  const [reloads, setReloads] = useState(0)
  // Ответ привязан к статусу: при смене фильтра старый список не показывается, items нет — ошибка загрузки.
  const [data, setData] = useState<{ status: string; items?: Promo[] }>()

  // Устаревший ответ при быстром переключении не перетирает новый; после «Выключить» — текущий фильтр.
  useEffect(() => {
    let stale = false
    get<{ items: Promo[] }>(apiStatus).then(
      (r) => {
        if (!stale) setData({ status: apiStatus, items: r.items })
      },
      () => {
        if (!stale) setData({ status: apiStatus })
      },
    )
    return () => {
      stale = true
    }
  }, [apiStatus, reloads])

  // раскрытая строка → её активации (undefined — грузятся, null — ошибка)
  const [open, setOpen] = useState<number | null>(null)
  const openRef = useRef<number | null>(null)
  const [acts, setActs] = useState<PromoActivation[] | null>()
  async function toggle(id: number) {
    const next = openRef.current === id ? null : id
    openRef.current = next
    setOpen(next)
    if (next === null) return
    setActs(undefined)
    try {
      const r = (await get<{ items: PromoActivation[] }>(`/api/promos/${id}/activations`)).items
      if (openRef.current === id) setActs(r)
    } catch {
      if (openRef.current === id) setActs(null)
    }
  }

  const [confirming, setConfirming] = useState<Promo>()
  const [deleting, setDeleting] = useState<Promo>()
  const toggling = useRef(false)
  async function setEnabled(p: Promo, enable: boolean) {
    setConfirming(undefined)
    if (toggling.current) return
    toggling.current = true
    try {
      await post(`/api/promos/${p.id}/${enable ? 'enable' : 'disable'}`)
      toast(enable ? 'Промокод включён' : `Промокод ${p.code} выключен`, { icon: 'check' })
      setReloads((n) => n + 1)
    } catch {
      toast(enable ? 'Не удалось включить промокод' : 'Не удалось выключить промокод')
    } finally {
      toggling.current = false
    }
  }

  const loaded = data?.status === apiStatus ? data : undefined
  const rows = loaded?.items ?? []
  const filtered = kind || who
  const empty = !loaded ? 'Загрузка…' : !loaded.items ? 'Не удалось загрузить промокоды' : filtered ? 'Ничего не найдено' : EMPTY[segment]

  const expanded = (p: Promo) =>
    open !== p.id ? null : (
      <div className="admin-table flex flex-col gap-2 bg-[color-mix(in_srgb,var(--page-bg)_55%,var(--surface))] px-5 py-3 whitespace-normal">
        {/* На телефоне в строке нет кнопок (кнопка в кнопке): действия и «Для кого» здесь. */}
        <div className="flex flex-col gap-2 md:hidden">
          <span>Для кого: {target(p)}</span>
          <div className="flex flex-wrap gap-2">
            <Button variant="gray" size="sm" icon="copy" onClick={() => void copyCode(p.code)}>
              Скопировать код
            </Button>
            {!p.deleted &&
              (p.disabled ? (
                <Button variant="gray" size="sm" onClick={() => void setEnabled(p, true)}>
                  Включить
                </Button>
              ) : (
                <Button variant="destructive" size="sm" onClick={() => setConfirming(p)}>
                  Выключить
                </Button>
              ))}
            {!p.deleted && (
              <Button variant="destructive" size="sm" onClick={() => setDeleting(p)}>
                Удалить
              </Button>
            )}
          </div>
        </div>
        <h3 className="admin-label text-text-secondary">Активации</h3>
        {acts === undefined ? (
          <Spinner size={20} />
        ) : acts === null ? (
          <p className="text-destructive-strong">Не удалось загрузить активации</p>
        ) : !acts.length ? (
          <p className="text-text-secondary">Ещё никто не активировал</p>
        ) : (
          <ul className="flex flex-col gap-1">
            {acts.map((a) => (
              <li key={a.id} className="break-words">
                {userLabel(a)} · {formatShortDate(a.createdAt)}
                {a.paymentId ? ` · платёж #${a.paymentId}` : ''}
              </li>
            ))}
          </ul>
        )}
      </div>
    )

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="max-w-full overflow-x-auto">
          <SegmentedControl options={SEGMENTS} value={segment} onChange={setSegment} />
        </div>
        <div className="grow" />
        <Button variant="gray" icon="filter" aria-expanded={showFilters} onClick={() => setShowFilters(!showFilters)}>
          Фильтры
        </Button>
        <Button icon="plus" onClick={() => navigate('/promos/new')}>
          Создать промокод
        </Button>
      </div>
      {showFilters && (
        <Card className="admin-body">
          <div className="flex flex-col gap-2">
            <span className="admin-label px-0.5 text-text-secondary">Тип</span>
            <SegmentedControl options={KIND_FILTER} value={kind || 'any'} onChange={(v) => setKind(v === 'any' ? '' : v)} />
          </div>
          <div className="flex flex-col gap-2">
            <span className="admin-label px-0.5 text-text-secondary">Для кого</span>
            <SegmentedControl options={TARGET_FILTER} value={who || 'any'} onChange={(v) => setWho(v === 'any' ? '' : v)} />
          </div>
        </Card>
      )}
      <div className="overflow-x-auto">
        <DataTable
          columns={columns}
          rows={rows}
          empty={empty}
          onRowClick={(p) => void toggle(p.id)}
          expandedRow={expanded}
          // У API нет страниц: весь список — одна страница.
          pagination={loaded?.items ? { page: 1, pages: 1, from: rows.length ? 1 : 0, to: rows.length, total: formatCount(rows.length) } : undefined}
          rowActions={(p) => (
            <span className="inline-flex gap-0.5">
              <RowAction icon="copy" label="Скопировать код" onClick={() => void copyCode(p.code)} />
              {!p.deleted && (
                <RowMenu
                  label="Действия с промокодом"
                  items={[
                    p.disabled
                      ? { label: 'Включить', onSelect: () => void setEnabled(p, true) }
                      : { label: 'Выключить', destructive: true, onSelect: () => setConfirming(p) },
                    { label: 'Удалить', destructive: true, onSelect: () => setDeleting(p) },
                  ]}
                />
              )}
            </span>
          )}
          mobileRow={(p) => ({
            title: <span className="font-mono font-semibold">{p.code}</span>,
            subtitle: [promoValue(p), `${formatCount(p.used)} / ${limitOf(p)}`, p.validUntil && promoUntil(p.validUntil)].filter(Boolean).join(' · '),
            after: statusTag(p),
            chevron: true,
          })}
        />
      </div>
      {confirming && (
        <Dialog
          title="Выключить промокод?"
          message={`Код ${confirming.code} перестанет работать. Его можно включить обратно.`}
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setConfirming(undefined) },
            { label: 'Выключить', style: 'destructive', onClick: () => void setEnabled(confirming, false) },
          ]}
        />
      )}
      {deleting && (
        <DeleteDialog
          title="Удалить промокод?"
          message={deleteMessage(deleting)}
          url={`/api/promos/${deleting.id}`}
          done="Промокод удалён"
          conflict="Не удалось удалить промокод"
          onClose={() => setDeleting(undefined)}
          onDeleted={() => {
            if (openRef.current === deleting.id) {
              openRef.current = null
              setOpen(null)
            }
            setReloads((n) => n + 1)
          }}
        />
      )}
    </>
  )
}
