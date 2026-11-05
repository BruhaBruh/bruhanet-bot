import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { notWired } from '@/lib/not-wired'
import { Button, DataTable, Dialog, RowAction, RowMenu, SegmentedControl, Spinner, StatusTag, toast, type DataTableColumn, type Tone } from '@/ui'
import { get, post } from '../api'
import { formatCount, formatDay, formatShortDate } from '../format'
import { kindLabels, promoStatus, promoUntil, promoValue, userLabel, type PromoStatus } from '../promo'
import type { Promo, PromoActivation } from '../types'

const SEGMENTS = [
  { value: 'all', label: 'Все' },
  { value: 'active', label: 'Активные' },
  { value: 'draft', label: 'Черновики' },
  { value: 'expired', label: 'Истёкшие' },
]
const EMPTY: Record<string, string> = {
  all: 'Промокодов ещё нет',
  active: 'Активных промокодов нет',
  draft: 'Черновиков нет',
  expired: 'Истёкших промокодов нет',
}

const STATUS: Record<PromoStatus, [Tone, string]> = {
  active: ['success', 'Активен'],
  expired: ['neutral', 'Истёк'],
  limit: ['warning', 'Лимит исчерпан'],
  disabled: ['neutral', 'Выключен'],
}

const statusTag = (p: Promo) => {
  const [tone, label] = STATUS[promoStatus(p)]
  return <StatusTag tone={tone}>{label}</StatusTag>
}

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
  const apiStatus = segment === 'active' ? 'active' : 'all'
  const [reloads, setReloads] = useState(0)
  // Ответ привязан к статусу: при смене фильтра старый список не показывается, items нет — ошибка загрузки.
  const [data, setData] = useState<{ status: string; items?: Promo[] }>()

  // Устаревший ответ при быстром переключении не перетирает новый; после «Выключить» — текущий фильтр.
  useEffect(() => {
    let stale = false
    get<{ items: Promo[] }>(`/api/promos?status=${apiStatus}`).then(
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
  const disabling = useRef(false)
  async function disable(p: Promo) {
    setConfirming(undefined)
    if (disabling.current) return
    disabling.current = true
    try {
      await post(`/api/promos/${p.id}/disable`)
      toast(`Промокод ${p.code} выключен`, { icon: 'check' })
      setReloads((n) => n + 1)
    } catch {
      toast('Не удалось выключить промокод')
    } finally {
      disabling.current = false
    }
  }

  const loaded = data?.status === apiStatus ? data : undefined
  const items = loaded?.items ?? []
  // ponytail: «Истёкшие» фильтруются на клиенте, у API только status=active|all; «Черновиков» нет в API и БД.
  const rows = segment === 'draft' ? [] : segment === 'expired' ? items.filter((p) => promoStatus(p) === 'expired') : items

  const empty = segment === 'draft' ? EMPTY.draft : !loaded ? 'Загрузка…' : !loaded.items ? 'Не удалось загрузить промокоды' : EMPTY[segment]

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
            {!p.disabled && (
              <Button variant="destructive" size="sm" onClick={() => setConfirming(p)}>
                Выключить
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
        <Button variant="gray" icon="filter" onClick={notWired}>
          Фильтры
        </Button>
        <Button icon="plus" onClick={() => navigate('/promos/new')}>
          Создать промокод
        </Button>
      </div>
      <div className="overflow-x-auto">
        <DataTable
          columns={columns}
          rows={rows}
          empty={empty}
          onRowClick={(p) => void toggle(p.id)}
          expandedRow={expanded}
          // У API нет страниц: весь список — одна страница.
          pagination={loaded?.items && segment !== 'draft' ? { page: 1, pages: 1, from: rows.length ? 1 : 0, to: rows.length, total: formatCount(rows.length) } : undefined}
          rowActions={(p) => (
            <span className="inline-flex gap-0.5">
              <RowAction icon="copy" label="Скопировать код" onClick={() => void copyCode(p.code)} />
              <RowMenu
                label="Действия с промокодом"
                items={[{ label: 'Выключить', destructive: true, disabled: p.disabled, onSelect: () => setConfirming(p) }]}
              />
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
          message={`Код ${confirming.code} перестанет работать. Включить его снова нельзя.`}
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setConfirming(undefined) },
            { label: 'Выключить', style: 'destructive', onClick: () => void disable(confirming) },
          ]}
        />
      )}
    </>
  )
}
