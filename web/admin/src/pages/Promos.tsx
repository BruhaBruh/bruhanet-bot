import { Button, Cell, Checkbox, Input, Placeholder, Radio, Section, SegmentedControl, Spinner, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, get, post } from '../api'
import { formatDate } from '../format'
import { generateCode, promoBody, promoGives, promoUntil, promoUsage, validatePromo, type PromoForm } from '../promo'
import type { Promo, PromoActivation } from '../types'

const filters = [
  { value: 'active', label: 'Активные' },
  { value: 'all', label: 'Все' },
] as const

const kinds = [
  { value: 'percent', label: 'скидка %', unit: '%' },
  { value: 'rub', label: 'скидка ₽', unit: '₽' },
  { value: 'days', label: 'бонусные дни', unit: 'дн' },
] as const

const empty = (): PromoForm => ({ code: '', kind: 'percent', value: '', maxActivations: '', validUntil: '', onlyNew: false, user: '' })

const marks = (p: Promo) =>
  [promoUsage(p), p.validUntil && promoUntil(p.validUntil), p.onlyNew && 'только новым', p.telegramId && (p.username ? '@' + p.username : `ID ${p.telegramId}`)]
    .filter(Boolean)
    .join(' · ')

const Loading = () => (
  <div className="tg-loading" role="status">
    <Spinner size="m" />
    <VisuallyHidden>Загрузка…</VisuallyHidden>
  </div>
)

export default function Promos() {
  const [status, setStatus] = useState<'active' | 'all'>('active')
  const [items, setItems] = useState<Promo[]>()
  const [loadError, setLoadError] = useState(false)
  const seq = useRef(0)
  // перезагрузка после действия — по фильтру на момент ответа, а не клика
  const statusRef = useRef(status)
  statusRef.current = status

  const load = useCallback(async (s: string) => {
    const n = ++seq.current // устаревший ответ при быстром переключении фильтра не перетирает новый
    try {
      const r = (await get<{ items: Promo[] }>(`/api/promos?status=${s}`)).items
      if (n !== seq.current) return
      setItems(r)
      setLoadError(false)
    } catch {
      if (n === seq.current) setLoadError(true)
    }
  }, [])
  useEffect(() => void load(status), [load, status])

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

  const [disabling, setDisabling] = useState<number | null>(null)
  const [disableError, setDisableError] = useState(false)
  async function disable(p: Promo) {
    if (disabling !== null) return
    setDisabling(p.id)
    setDisableError(false)
    try {
      await post(`/api/promos/${p.id}/disable`)
      await load(statusRef.current)
    } catch {
      setDisableError(true)
    } finally {
      setDisabling(null)
    }
  }

  const [form, setForm] = useState(empty)
  const set = (p: Partial<PromoForm>) => setForm((f) => ({ ...f, ...p }))
  const [busy, setBusy] = useState(false)
  const inFlight = useRef(false)
  const [error, setError] = useState('')
  const [created, setCreated] = useState('')

  async function create() {
    if (inFlight.current) return
    setCreated('')
    const msg = validatePromo(form)
    setError(msg)
    if (msg) return
    inFlight.current = true
    setBusy(true)
    try {
      const { promo } = await post<{ promo: Promo }>('/api/promos', promoBody(form))
      setCreated(`Создан ${promo.code}`)
      setForm(empty())
      await load(statusRef.current)
    } catch (e) {
      setError(e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось создать, попробуйте ещё раз')
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  return (
    <>
      <VisuallyHidden Component="h1">Промокоды</VisuallyHidden>
      <SegmentedControl className="promo-filter" aria-label="Фильтр">
        {filters.map((f) => (
          <SegmentedControl.Item key={f.value} selected={status === f.value} aria-selected={status === f.value} onClick={() => setStatus(f.value)}>
            {f.label}
          </SegmentedControl.Item>
        ))}
      </SegmentedControl>
      {disableError && (
        <p className="tg-err pad" role="alert">
          Не удалось выключить
        </p>
      )}
      {loadError ? (
        <p className="tg-err pad" role="alert">
          Не удалось загрузить
        </p>
      ) : !items ? (
        <Loading />
      ) : !items.length ? (
        <Placeholder header="Промокодов нет">
          <span className="ph-emoji">🎟</span>
        </Placeholder>
      ) : (
        <Section>
          {items.map((p) => (
            <div key={p.id}>
              {/* строка и «Выключить» — соседи: кнопка не вложена в кнопку */}
              <div className="promo-row">
                <Cell Component="button" className="promo-cell" multiline aria-expanded={open === p.id} subtitle={`${promoGives(p)} · ${marks(p)}`} onClick={() => void toggle(p.id)}>
                  {p.code}
                </Cell>
                {p.disabled ? (
                  <span className="promo-off">выключен</span>
                ) : (
                  <Button mode="plain" size="s" loading={disabling === p.id} onClick={() => void disable(p)}>
                    Выключить
                  </Button>
                )}
              </div>
              {open === p.id && (
                <div className="promo-acts">
                  {acts === undefined ? (
                    <Spinner size="s" />
                  ) : acts === null ? (
                    <p className="tg-err">Не удалось загрузить</p>
                  ) : !acts.length ? (
                    <p>Ещё никто не активировал</p>
                  ) : (
                    acts.map((a) => (
                      <p key={a.id}>
                        {a.username ? '@' + a.username : a.telegramId} · {formatDate(a.createdAt)}
                        {a.paymentId ? ` · платёж #${a.paymentId}` : ''}
                      </p>
                    ))
                  )}
                </div>
              )}
            </div>
          ))}
        </Section>
      )}

      <form
        className="promo-form"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void create()
        }}
      >
        <Section header="Новый промокод" footer="Пустой код сгенерирует сервер.">
          <div className="field-row">
            <Input placeholder="Код, например SUMMER25" aria-label="Код" value={form.code} onChange={(e) => set({ code: e.target.value })} />
            <Button type="button" mode="plain" aria-label="Сгенерировать код" onClick={() => set({ code: generateCode() })}>
              🎲
            </Button>
          </div>
          <div role="radiogroup" aria-label="Тип">
            {kinds.map((k) => (
              <Cell key={k.value} Component="label" before={<Radio name="kind" value={k.value} checked={form.kind === k.value} onChange={() => set({ kind: k.value })} />}>
                {k.label}
              </Cell>
            ))}
          </div>
          <Input
            type="number"
            min={1}
            placeholder="Значение"
            aria-label="Значение"
            after={kinds.find((k) => k.value === form.kind)!.unit}
            value={form.value}
            onChange={(e) => set({ value: e.target.value })}
          />
        </Section>
        <Section header="Ограничения (необязательно)">
          <Input type="number" min={1} placeholder="Макс. активаций" aria-label="Макс. активаций" value={form.maxActivations} onChange={(e) => set({ maxActivations: e.target.value })} />
          <Input type="date" before="до" aria-label="Действует до (включительно)" value={form.validUntil} onChange={(e) => set({ validUntil: e.target.value })} />
          <Cell Component="label" multiline before={<Checkbox checked={form.onlyNew} onChange={(e) => set({ onlyNew: e.target.checked })} />}>
            только новым (ни одной оплаты)
          </Cell>
          <Input placeholder="Только для юзера: @username или ID" aria-label="Только для юзера" value={form.user} onChange={(e) => set({ user: e.target.value })} />
        </Section>
        {error && (
          <p className="tg-err pad" role="alert">
            {error}
          </p>
        )}
        {created && (
          <p className="tg-ok pad" role="status">
            {created}
          </p>
        )}
        <Button className="settings-save" type="submit" loading={busy}>
          Создать
        </Button>
      </form>
    </>
  )
}
