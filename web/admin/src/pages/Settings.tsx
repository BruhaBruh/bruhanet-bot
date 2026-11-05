import { useEffect, useRef, useState } from 'react'
import { NOT_WIRED } from '@/lib/not-wired'
import { Button, Card, Icon, Input, Spinner, toast } from '@/ui'
import { ApiError, get, put } from '../api'
import { formatRub } from '../format'
import type { Settings as S } from '../types'

type Tariff = { months: string; price: string }
type Form = { tariffs: Tariff[]; phone: string; bank: string; comments: string[]; supportUrl: string }

const toForm = (s: S): Form => ({
  tariffs: Object.entries(s.prices)
    .sort(([a], [b]) => Number(a) - Number(b))
    .map(([months, price]) => ({ months, price: String(price) })),
  phone: s.phone,
  bank: s.bank,
  comments: [...s.comments],
  supportUrl: s.supportUrl,
})

function perMonth(t: Tariff, first: Tariff, i: number): string {
  const months = Number(t.months)
  if (!(months > 0) || !t.price) return '—'
  const cost = Number(t.price) / months
  const base = Number(first.price) / Number(first.months)
  const profit = i > 0 && Number.isFinite(base) && base > 0 ? ` · выгода ${Math.max(0, Math.round(100 - (cost / base) * 100))}%` : ''
  return `${formatRub(Math.round(cost))} / мес.${profit}`
}

// Текст ошибки, если тарифы нельзя отправить.
function tariffsError(tariffs: Tariff[]): string {
  // одинаковые сроки схлопнулись бы в один ключ объекта — ловим до отправки
  const dup = tariffs.map((t) => t.months.trim()).find((m, i, a) => a.indexOf(m) !== i)
  if (dup !== undefined) return `Тариф на ${dup || '(пусто)'} мес. указан дважды`
  const bad = tariffs.some((t) => !/^\d+$/.test(t.months.trim()) || +t.months < 1 || +t.months > 120 || !/^\d+$/.test(t.price.trim()) || +t.price < 1)
  return bad ? 'Срок тарифа — целое число от 1 до 120 месяцев, цена — целое число больше 0' : ''
}

const removeButton = 'w-10 flex-none px-0'

export default function Settings() {
  const [form, setForm] = useState<Form>()
  const [saved, setSaved] = useState<Form>()
  const [stored, setStored] = useState<string[] | null>(null)
  const [loadError, setLoadError] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [problems, setProblems] = useState<string[]>([])
  const inFlight = useRef(false)

  useEffect(() => {
    get<S>('/api/settings').then(
      (s) => {
        const loaded = toForm(s)
        setForm(loaded)
        setSaved(loaded)
        if (s.usingDefaults) setStored(s.problems ?? [])
      },
      () => setLoadError(true),
    )
  }, [])

  if (loadError)
    return (
      <p role="alert" className="admin-body text-destructive-strong">
        Не удалось загрузить
      </p>
    )
  if (!form) return <Spinner className="self-center p-8" />

  const patch = (p: Partial<Form>) => setForm({ ...form, ...p })
  const setTariff = (i: number, p: Partial<Tariff>) => patch({ tariffs: form.tariffs.map((t, j) => (j === i ? { ...t, ...p } : t)) })
  const pristine = JSON.stringify(form) === JSON.stringify(saved)
  const supportError = form.supportUrl && !/^https?:\/\//.test(form.supportUrl) ? 'Ссылка должна начинаться с https://' : undefined
  // Некорректные сохранённые настройки чинятся сохранением даже без правок: бот уже работает на значениях из формы.
  const dirty = !pristine || !!stored

  function reset() {
    setForm(saved)
    setError('')
    setProblems([])
  }

  async function save() {
    if (inFlight.current || !form) return
    setProblems([])
    const invalid = tariffsError(form.tariffs)
    setError(invalid)
    if (invalid) return
    const body: S = {
      prices: Object.fromEntries(form.tariffs.map((t) => [t.months.trim(), Number(t.price)])),
      phone: form.phone,
      bank: form.bank,
      comments: form.comments,
      supportUrl: form.supportUrl,
    }
    inFlight.current = true
    setBusy(true)
    try {
      await put('/api/settings', body)
      setSaved(form)
      setStored(null)
      toast('Настройки сохранены', { icon: 'check' })
    } catch (e) {
      if (e instanceof ApiError && e.body.message) {
        setError(e.body.message)
        setProblems(e.body.problems ?? [])
      } else setError('Не удалось сохранить, попробуйте ещё раз')
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  return (
    <form
      noValidate
      className="admin-body flex max-w-215 flex-col gap-4 text-text"
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      {stored && (
        <div role="alert" className="flex flex-col gap-1 rounded-lg bg-warning-tint p-4">
          <span className="headline">Сохранённые настройки некорректны</span>
          <span>Бот работает на значениях из .env: {stored.join('; ')}.</span>
          <span>Сохраните форму, чтобы применить новые.</span>
        </div>
      )}

      <Card title="Тарифы" aside="пользователь выбирает срок при оплате" className="gap-2.5">
        <div className="admin-label hidden gap-2 px-0.5 text-text-secondary md:flex">
          <span className="grow basis-40">Срок</span>
          <span className="grow basis-45">Цена</span>
          <span className="grow basis-35">В месяц</span>
          <span className="w-10 flex-none" />
        </div>
        {form.tariffs.map((t, i) => (
          <div key={i} className="flex flex-wrap items-center gap-2 md:flex-nowrap">
            <Input className="min-w-0 grow basis-40" type="number" min={1} max={120} aria-label="Срок в месяцах" after="мес." value={t.months} onChange={(e) => setTariff(i, { months: e.target.value })} />
            <Input className="min-w-0 grow basis-45" type="number" min={1} aria-label="Цена" after="₽" value={t.price} onChange={(e) => setTariff(i, { price: e.target.value })} />
            <span className="min-w-0 grow basis-35 text-text-secondary">{perMonth(t, form.tariffs[0], i)}</span>
            <Button
              variant="gray"
              icon="trash"
              className={removeButton}
              aria-label="Удалить тариф"
              disabled={form.tariffs.length <= 1}
              onClick={() => patch({ tariffs: form.tariffs.filter((_, j) => j !== i) })}
            />
          </div>
        ))}
        <div>
          <Button variant="tinted" icon="plus" onClick={() => patch({ tariffs: [...form.tariffs, { months: '', price: '' }] })}>
            Добавить тариф
          </Button>
        </div>
      </Card>

      <Card title="Устройства">
        <div className="flex flex-wrap gap-3">
          <Input className="min-w-0 grow basis-60" label="Устройств по умолчанию" type="number" disabled readOnly value="" after="шт." hint={NOT_WIRED} />
          <Input className="min-w-0 grow basis-60" label="Цена дополнительного устройства" type="number" disabled readOnly value="" after="₽ / мес." hint={NOT_WIRED} />
        </div>
      </Card>

      <Card title="Реквизиты для перевода">
        <div className="flex flex-wrap gap-3">
          <Input className="min-w-0 grow basis-60" label="Номер телефона" type="tel" placeholder="+7 900 000-00-00" value={form.phone} onChange={(e) => patch({ phone: e.target.value })} />
          <Input className="min-w-0 grow basis-60" label="Банк" placeholder="Например, Т-Банк" value={form.bank} onChange={(e) => patch({ bank: e.target.value })} />
        </div>
        <div className="admin-table flex items-center gap-2.5 rounded-md bg-page-bg px-3 py-2.5">
          <Icon name="card" className="flex-none text-text-secondary" />
          <span className="min-w-0 wrap-anywhere">
            Пользователь увидит: <b>{`${form.phone || '—'} · ${form.bank || '—'}`}</b>
          </span>
        </div>
      </Card>

      <Card title="Комментарии к переводу" className="gap-2.5">
        {form.comments.map((c, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input
              className="min-w-0 grow"
              placeholder="Комментарий"
              aria-label="Комментарий"
              value={c}
              onChange={(e) => patch({ comments: form.comments.map((x, j) => (j === i ? e.target.value : x)) })}
            />
            <Button
              variant="gray"
              icon="trash"
              className={removeButton}
              aria-label="Удалить комментарий"
              onClick={() => patch({ comments: form.comments.filter((_, j) => j !== i) })}
            />
          </div>
        ))}
        <div>
          <Button variant="tinted" icon="plus" onClick={() => patch({ comments: [...form.comments, ''] })}>
            Добавить комментарий
          </Button>
        </div>
      </Card>

      <Card title="Поддержка">
        <Input
          className="max-w-105"
          label="Ссылка на поддержку"
          type="url"
          value={form.supportUrl}
          onChange={(e) => patch({ supportUrl: e.target.value })}
          hint="Открывается по кнопке «Поддержка» в боте"
          error={supportError}
        />
      </Card>

      {error && (
        <div role="alert" className="text-destructive-strong">
          {problems.length <= 1 ? (
            error
          ) : (
            <ul className="list-disc pl-5">
              {problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      <div className="flex flex-wrap items-center justify-end gap-3">
        <span className="footnote grow text-text-secondary">{dirty ? 'Есть несохранённые изменения' : 'Все изменения сохранены'}</span>
        <Button variant="gray" disabled={pristine || busy} onClick={reset}>
          Отменить
        </Button>
        <Button type="submit" loading={busy} disabled={!dirty || !!supportError}>
          Сохранить изменения
        </Button>
      </div>
    </form>
  )
}
