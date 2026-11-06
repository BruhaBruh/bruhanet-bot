import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { NOT_WIRED } from '@/lib/not-wired'
import { Avatar, Button, Card, IconTile, Input, SearchField, SegmentedControl, Switch, toast, type IconName } from '@/ui'
import { ApiError, get, post } from '../api'
import { formatCount, formatDay, mskDate, plural } from '../format'
import { generateCode, kindLabels, promoBody, promoValue, userLabel, validatePromo, type PromoKind } from '../promo'
import type { Promo, UserSummary } from '../types'

const KIND_OPTIONS = (['percent', 'rub', 'days'] as const).map((value) => ({ value, label: kindLabels[value] }))
const UNITS: Record<PromoKind, string> = { percent: '%', rub: '₽', days: 'дн.' }
const ICONS: Record<PromoKind, IconName> = { percent: 'percent', rub: 'ruble', days: 'gift' }
const DEFAULT_VALUES: Record<PromoKind, string> = { percent: '20', rub: '300', days: '7' }

type Target = 'all' | 'new' | 'user'
const TARGET_OPTIONS: { value: Target; label: string }[] = [
  { value: 'all', label: 'Для всех' },
  { value: 'new', label: 'Для новых' },
  { value: 'user', label: 'Конкретному пользователю' },
]
const TARGET_HINTS: Record<Target, string> = {
  all: 'Код сможет применить любой пользователь бота.',
  new: 'Только пользователи, которые ещё ни разу не оплачивали подписку.',
  user: 'Код сработает только у выбранного пользователя — остальным бот ответит, что промокод не действует.',
}

const USERS_SHOWN = 5
const note = 'footnote text-text-secondary'
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'
const CODE_HINT = 'promo-code-hint'

export default function PromoNew() {
  const navigate = useNavigate()
  const [code, setCode] = useState('')
  const [kind, setKind] = useState<PromoKind>('percent')
  const [value, setValue] = useState('')
  const [limitOn, setLimitOn] = useState(false)
  const [limit, setLimit] = useState('')
  const [termOn, setTermOn] = useState(false)
  const [until, setUntil] = useState('')
  const [target, setTarget] = useState<Target>('all')
  const [picked, setPicked] = useState<UserSummary>()
  const [query, setQuery] = useState('')
  const [found, setFound] = useState<{ items?: UserSummary[] }>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  // синхронный флаг: второй клик до перерисовки не шлёт второй запрос
  const inFlight = useRef(false)

  const searching = target === 'user' && !picked
  // Устаревший ответ не перетирает новый, debounce 300 мс.
  useEffect(() => {
    if (!searching) return
    let stale = false
    const timer = setTimeout(() => {
      get<{ items: UserSummary[] }>(`/api/users?q=${encodeURIComponent(query.trim())}`).then(
        (r) => {
          if (!stale) setFound({ items: r.items.slice(0, USERS_SHOWN) })
        },
        () => {
          if (!stale) setFound({})
        },
      )
    }, 300)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  }, [searching, query])

  const n = Number(value)
  const valueError = value && kind === 'percent' && (n < 1 || n > 100) ? 'От 1 до 100%' : undefined
  const cantCreate = !value || !!valueError || (limitOn && !limit.trim()) || (termOn && !until) || (target === 'user' && !picked)

  async function create() {
    if (inFlight.current || cantCreate) return
    const form = {
      code,
      kind,
      value,
      maxActivations: limitOn ? limit : '',
      validUntil: termOn ? until : '',
      onlyNew: target === 'new',
      user: target === 'user' && picked ? String(picked.telegramId) : '',
    }
    const message = validatePromo(form)
    setError(message)
    if (message) return
    inFlight.current = true
    setBusy(true)
    try {
      const { promo } = await post<{ promo: Promo }>('/api/promos', promoBody(form))
      toast(`Промокод ${promo.code} создан`, { icon: 'check' })
      navigate('/promos', { replace: true })
    } catch (e) {
      setError(e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось создать, попробуйте ещё раз')
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  const limitCount = formatCount(Number(limit) || 0)
  const untilLabel = until && formatDay(until)
  const summary = [
    { k: 'Тип', v: kindLabels[kind] },
    { k: 'Активации', v: limitOn ? `${limitCount} ${plural(Number(limit) || 0, ['раз', 'раза', 'раз'])}` : 'без ограничений' },
    { k: 'Срок', v: !termOn ? 'бессрочно' : untilLabel ? `по ${untilLabel}` : 'дата не выбрана' },
    {
      k: 'Для кого',
      v: target === 'all' ? 'Все пользователи' : target === 'new' ? 'Только новые' : picked ? userLabel(picked) : 'Пользователь не выбран',
    },
  ]

  return (
    <>
      <div className="-my-2 -ml-2">
        <Button variant="plain" icon="chevron-left" onClick={() => navigate('/promos')}>
          Все промокоды
        </Button>
      </div>
      <div className="admin-body flex flex-wrap items-start gap-6">
        <form
          noValidate
          className="flex min-w-0 grow-999 basis-130 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            void create()
          }}
        >
          <Card title="Промокод" className="gap-4">
            <div className="flex flex-col gap-1.5">
              <div className="flex flex-wrap items-end gap-2">
                <Input
                  className="flex-[1_1_--spacing(65)]"
                  label="Код"
                  placeholder="Например, SALE20"
                  aria-describedby={CODE_HINT}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
                <Button variant="gray" icon="refresh" onClick={() => setCode(generateCode())}>
                  Сгенерировать
                </Button>
              </div>
              <p id={CODE_HINT} className={`${note} px-0.5`}>
                Латиница, цифры и дефис — пользователь вводит его в боте. Пустое поле — код создаст сервер.
              </p>
            </div>
            <div className="flex flex-col gap-2">
              <span className="admin-label px-0.5 text-text-secondary">Тип</span>
              <div className="overflow-x-auto">
                <SegmentedControl
                  block
                  options={KIND_OPTIONS}
                  value={kind}
                  onChange={(v) => {
                    const next = v as PromoKind
                    setKind(next)
                    setValue(DEFAULT_VALUES[next])
                  }}
                />
              </div>
            </div>
            <Input
              className="max-w-70"
              label={kind === 'days' ? 'Сколько дней подписки добавить' : 'Размер скидки'}
              type="number"
              fit
              min={1}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              after={UNITS[kind]}
              error={valueError}
            />
          </Card>

          <Card className="gap-0 px-5 py-1">
            <div className={`flex min-h-14 items-center gap-3 ${separator}`}>
              <div className="flex grow flex-col">
                <span>Ограничить количество активаций</span>
                <span className={note}>
                  {limitOn
                    ? `после ${limitCount} ${plural(Number(limit) || 0, ['активации', 'активаций', 'активаций'])} код перестанет работать`
                    : 'сейчас — без ограничений'}
                </span>
              </div>
              <Switch checked={limitOn} onChange={setLimitOn} aria-label="Ограничить количество активаций" />
            </div>
            {limitOn && (
              <div className={`py-3 ${separator}`}>
                <Input
                  className="max-w-70"
                  label="Сколько раз можно применить"
                  type="number"
                  fit
                  min={1}
                  value={limit}
                  onChange={(e) => setLimit(e.target.value)}
                  after="раз"
                />
              </div>
            )}
            <div className="flex min-h-14 items-center gap-3">
              <div className="flex grow flex-col">
                <span>Ограничить срок действия</span>
                <span className={note}>{!termOn ? 'сейчас — бессрочно' : untilLabel ? `по ${untilLabel} включительно` : 'дата не выбрана'}</span>
              </div>
              <Switch checked={termOn} onChange={setTermOn} aria-label="Ограничить срок действия" />
            </div>
            {termOn && (
              <div className="flex flex-wrap gap-3 pb-4">
                <Input className="flex-[0_1_--spacing(55)]" label="Действует с" type="date" disabled hint={NOT_WIRED} />
                <Input
                  className="flex-[0_1_--spacing(55)]"
                  label="Действует по"
                  type="date"
                  min={mskDate(new Date())}
                  hint="Включительно, по Москве"
                  value={until}
                  onChange={(e) => setUntil(e.target.value)}
                />
              </div>
            )}
          </Card>

          <Card title="Для кого">
            <div className="overflow-x-auto">
              <SegmentedControl block options={TARGET_OPTIONS} value={target} onChange={(v) => setTarget(v as Target)} />
            </div>
            <p className={note}>{TARGET_HINTS[target]}</p>
            {target === 'user' && picked && (
              <div className="flex items-center gap-3 rounded-md bg-accent-tint px-3 py-2.5">
                <Avatar name={userLabel(picked)} id={picked.telegramId} size={32} />
                <div className="flex min-w-0 grow flex-col">
                  <span className="truncate font-semibold">{userLabel(picked)}</span>
                  <span className={`${note} truncate`}>{picked.username ? `@${picked.username} · ` : ''}ID {picked.telegramId}</span>
                </div>
                <Button size="sm" variant="gray" onClick={() => setPicked(undefined)}>
                  Изменить
                </Button>
              </div>
            )}
            {searching && (
              <>
                <SearchField
                  placeholder="@username или Telegram ID"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  onClear={() => setQuery('')}
                />
                {found && !found.items && (
                  <p role="alert" className="text-destructive-strong">
                    Не удалось найти пользователей
                  </p>
                )}
                {found?.items &&
                  (found.items.length ? (
                    <div className="flex flex-col overflow-hidden rounded-md shadow-[inset_0_0_0_0.5px_var(--separator)]">
                      {found.items.map((u) => (
                        <button
                          key={u.telegramId}
                          type="button"
                          className={`focus-ring flex cursor-pointer items-center gap-3 bg-surface px-3 py-2 text-left text-text focus-visible:-outline-offset-2 ${separator}`}
                          onClick={() => {
                            setPicked(u)
                            setQuery('')
                          }}
                        >
                          <Avatar name={userLabel(u)} id={u.telegramId} size={28} />
                          <span className="min-w-0 grow truncate">{userLabel(u)}</span>
                          {u.username && <span className={`${note} flex-none`}>ID {u.telegramId}</span>}
                        </button>
                      ))}
                    </div>
                  ) : (
                    <p className={note}>Никого не нашли</p>
                  ))}
              </>
            )}
          </Card>

          {error && (
            <p role="alert" className="text-destructive-strong">
              {error}
            </p>
          )}

          <div className="flex flex-wrap justify-end gap-3">
            <Button variant="gray" onClick={() => navigate('/promos')}>
              Отмена
            </Button>
            <Button type="submit" loading={busy} disabled={cantCreate}>
              Создать промокод
            </Button>
          </div>
        </form>

        <aside aria-label="Итог" className="flex max-w-95 min-w-0 grow basis-75 flex-col gap-2 md:sticky md:top-6">
          <h2 className="admin-label px-1 text-text-secondary">Итог</h2>
          <div className="flex flex-col gap-3.5 rounded-lg bg-surface p-5 shadow-card">
            <div className="flex items-center gap-3">
              <IconTile icon={ICONS[kind]} color="orange" />
              <code className="title-3 min-w-0 truncate font-mono font-bold">{code.trim().toUpperCase() || 'КОД'}</code>
            </div>
            <div className="admin-stat">{!value ? '—' : promoValue({ kind, value: n }) + (kind === 'days' ? ' подписки' : '')}</div>
            <div className="flex flex-col">
              {summary.map((r) => (
                <div key={r.k} className="flex justify-between gap-3 py-2 shadow-[inset_0_0.5px_0_var(--separator)]">
                  <span className="text-text-secondary">{r.k}</span>
                  <span className="min-w-0 text-right font-medium break-words">{r.v}</span>
                </div>
              ))}
            </div>
          </div>
        </aside>
      </div>
    </>
  )
}
