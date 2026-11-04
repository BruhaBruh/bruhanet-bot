import { Banner, Button, Input, Section, Spinner, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useEffect, useRef, useState } from 'react'
import { ApiError, get, put } from '../api'
import type { Settings as S } from '../types'

type Tariff = { months: string; price: string }

export default function Settings() {
  const [tariffs, setTariffs] = useState<Tariff[]>([])
  const [phone, setPhone] = useState('')
  const [bank, setBank] = useState('')
  const [comments, setComments] = useState<string[]>([])
  const [supportUrl, setSupportUrl] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const [busy, setBusy] = useState(false)
  const inFlight = useRef(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState('')
  const [problems, setProblems] = useState<string[]>([])
  const [stored, setStored] = useState<string[] | null>(null)

  useEffect(() => {
    get<S>('/api/settings').then(
      (s) => {
        setTariffs(
          Object.entries(s.prices)
            .sort(([a], [b]) => Number(a) - Number(b))
            .map(([months, price]) => ({ months, price: String(price) })),
        )
        setPhone(s.phone)
        setBank(s.bank)
        setComments([...s.comments])
        setSupportUrl(s.supportUrl)
        if (s.usingDefaults) setStored(s.problems ?? [])
        setLoaded(true)
      },
      () => setLoadError(true),
    )
  }, [])

  async function save() {
    if (inFlight.current) return
    setError('')
    setProblems([])
    setSaved(false)
    // одинаковые сроки схлопнулись бы в один ключ объекта — ловим до отправки
    const dup = tariffs.map((t) => t.months.trim()).find((m, i, a) => a.indexOf(m) !== i)
    if (dup !== undefined) return setError(`Тариф на ${dup || '(пусто)'} мес. указан дважды`)
    const bad = tariffs.find((t) => !/^\d+$/.test(t.months.trim()) || +t.months < 1 || +t.months > 120 || !/^\d+$/.test(t.price.trim()) || +t.price < 1)
    if (bad) return setError('Срок тарифа — целое число от 1 до 120 месяцев, цена — целое число больше 0')
    const body: S = {
      prices: Object.fromEntries(tariffs.map((t) => [t.months.trim(), Number(t.price)])),
      phone,
      bank,
      comments,
      supportUrl,
    }
    inFlight.current = true
    setBusy(true)
    try {
      await put('/api/settings', body)
      setSaved(true)
      setStored(null)
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

  if (loadError)
    return (
      <p className="tg-err pad" role="alert">
        Не удалось загрузить
      </p>
    )
  if (!loaded)
    return (
      <div className="tg-loading" role="status">
        <Spinner size="m" />
        <VisuallyHidden>Загрузка…</VisuallyHidden>
      </div>
    )

  const setTariff = (i: number, p: Partial<Tariff>) => setTariffs((ts) => ts.map((t, j) => (j === i ? { ...t, ...p } : t)))

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
      onInput={() => setSaved(false)}
    >
      <VisuallyHidden Component="h1">Настройки</VisuallyHidden>
      {stored && (
        <Banner
          className="settings-banner"
          type="section"
          header="Сохранённые настройки некорректны"
          subheader={`Бот работает на значениях из .env: ${stored.join('; ')}.`}
          description="Сохраните форму, чтобы применить новые."
          role="alert"
        />
      )}

      <Section header="Тарифы" footer="Месяцев и цена в рублях. Хотя бы один тариф.">
        {tariffs.map((t, i) => (
          <div key={i} className="field-row">
            <Input type="number" min={1} placeholder="Месяцев" aria-label="Месяцев" after="мес" value={t.months} onChange={(e) => setTariff(i, { months: e.target.value })} />
            <Input type="number" min={1} placeholder="Цена" aria-label="Цена, ₽" after="₽" value={t.price} onChange={(e) => setTariff(i, { price: e.target.value })} />
            <Button type="button" mode="plain" disabled={tariffs.length <= 1} aria-label="Удалить тариф" onClick={() => setTariffs((ts) => ts.filter((_, j) => j !== i))}>
              ✕
            </Button>
          </div>
        ))}
      </Section>
      <Button className="settings-add" type="button" mode="bezeled" size="s" onClick={() => setTariffs((ts) => [...ts, { months: '', price: '' }])}>
        Добавить тариф
      </Button>

      <Section header="Реквизиты">
        <Input placeholder="Телефон" aria-label="Телефон" value={phone} onChange={(e) => setPhone(e.target.value)} />
        <Input placeholder="Банк" aria-label="Банк" value={bank} onChange={(e) => setBank(e.target.value)} />
      </Section>

      <Section header="Комментарии к переводу">
        {comments.map((c, i) => (
          <div key={i} className="field-row">
            <Input placeholder="Комментарий" aria-label="Комментарий" value={c} onChange={(e) => setComments((cs) => cs.map((x, j) => (j === i ? e.target.value : x)))} />
            <Button type="button" mode="plain" aria-label="Удалить комментарий" onClick={() => setComments((cs) => cs.filter((_, j) => j !== i))}>
              ✕
            </Button>
          </div>
        ))}
      </Section>
      <Button className="settings-add" type="button" mode="bezeled" size="s" onClick={() => setComments((cs) => [...cs, ''])}>
        Добавить комментарий
      </Button>

      <Section header="Поддержка">
        <Input type="url" placeholder="https://t.me/…" aria-label="Ссылка на поддержку" value={supportUrl} onChange={(e) => setSupportUrl(e.target.value)} />
      </Section>

      {error && (
        <div className="tg-err pad" role="alert">
          {problems.length <= 1 ? (
            error
          ) : (
            <ul>
              {problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
          )}
        </div>
      )}
      {saved && (
        <p className="tg-ok pad" role="status">
          Сохранено
        </p>
      )}
      <Button className="settings-save" type="submit" loading={busy}>
        Сохранить
      </Button>
    </form>
  )
}
