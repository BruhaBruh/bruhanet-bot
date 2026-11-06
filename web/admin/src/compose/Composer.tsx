import { useEffect, useRef, useState } from 'react'
import { BOT_NAME, OWNER_USERNAME } from '@/brand'
import { cn } from '@/lib/utils'
import { NOT_WIRED } from '@/lib/not-wired'
import { Avatar, Button, Card, Dialog, Icon, Input, MessageBubble, Radio, RadioGroup, Switch, toast } from '@/ui'
import { ApiError, get, post } from '../api'
import { previewHtml, renderPreview, varLabel, visibleText, worstLen } from '../editor/serialize'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { VarChips } from '../editor/VarChips'
import { formatCount, plural } from '../format'
import type { Audience, Broadcast, DirectMessage, TextVar } from '../types'
import { AUDIENCES, audienceLabel, daysValid, DEFAULT_DAYS, MAX_DAYS, MIN_DAYS } from './audience'
import { PhotoPicker } from './PhotoPicker'

export interface ComposerDraft {
  text: string
  photo?: string
  audience?: Audience
  audienceDays?: number | null
}

// broadcast — всем из панели; direct — одному юзеру (userId)
export type ComposerProps =
  | { mode: 'broadcast'; initial?: ComposerDraft | null; onSent?(b: Broadcast): void }
  | { mode: 'direct'; userId: number; onSent?(m: DirectMessage): void }

type Counts = Partial<Record<Audience, number>>
// Цвета стилей кнопок — как в макете, без бэкенда только образец.
const BUTTON_STYLES = [
  { label: 'Обычная', className: 'bg-keyboard-btn-bg shadow-[inset_0_0_0_1px_var(--separator)]' },
  { label: 'Основная (синяя)', className: 'bg-accent-strong' },
  { label: 'Успех (зелёная)', className: 'bg-success-strong' },
  { label: 'Опасная (красная)', className: 'bg-destructive-strong' },
]

const note = 'footnote text-text-secondary'
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'

const NotWiredHint = () => <p className={cn(note, 'py-2')}>{NOT_WIRED}</p>

function Toggle({ title, subtitle, last }: { title: string; subtitle: string; last?: boolean }) {
  return (
    <div className={cn('flex min-h-14 items-center gap-3', !last && separator)} title={NOT_WIRED}>
      <div className="flex grow flex-col">
        <span>{title}</span>
        <span className={note}>{subtitle}</span>
      </div>
      <Switch disabled aria-label={title} />
    </div>
  )
}

// Все аудитории сразу (expiring — за days дней); undefined — не удалось получить
const fetchCounts = (days: string) =>
  get<Counts>(`/api/broadcasts/recipients${daysValid(days) ? `?days=${days}` : ''}`).then(
    (r) => r,
    () => undefined,
  )

// Одна аудитория на момент клика; undefined — не удалось получить
const fetchRecipients = (audience: Audience, days: string) =>
  get<{ count: number }>(`/api/broadcasts/recipients?audience=${audience}${audience === 'expiring' ? `&days=${days}` : ''}`).then(
    (r) => r.count,
    () => undefined,
  )

export function Composer(props: ComposerProps) {
  const { mode } = props
  const isBroadcast = mode === 'broadcast'
  const initial = props.mode === 'broadcast' ? props.initial : undefined
  const [text, setText] = useState(initial?.text ?? '')
  const [photo, setPhoto] = useState(initial?.photo)
  const [photoUrl, setPhotoUrl] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [audience, setAudience] = useState<Audience>(initial?.audience ?? 'all')
  const [days, setDays] = useState(initial?.audienceDays ? String(initial.audienceDays) : DEFAULT_DAYS)
  // null — грузится, undefined — не удалось получить
  const [counts, setCounts] = useState<Counts | null | undefined>(null)
  const [uploading, setUploading] = useState(false)
  const [globals, setGlobals] = useState<TextVar[]>([])
  const editorRef = useRef<TgEditorHandle>(null)
  // ключ идемпотентности — на конкретное содержимое: повтор без правок после сбоя
  // не создаст вторую рассылку, а правка даёт новый ключ (иначе сервер вернёт старую)
  const [firstKey] = useState(() => crypto.randomUUID())
  const key = useRef(firstKey)
  // синхронный флаг: второй клик до перерисовки не шлёт второй запрос
  const inFlight = useRef(false)

  const daysOk = daysValid(days)
  useEffect(() => {
    if (isBroadcast) void fetchCounts(daysOk ? days : '').then(setCounts)
  }, [isBroadcast, days, daysOk])

  // Не загрузились — нет чипов и примеров, сервер всё равно проверит текст.
  useEffect(() => {
    get<{ globals: TextVar[] }>('/api/texts').then(
      (r) => setGlobals(r.globals),
      () => {},
    )
  }, [])

  // правка черновика (и сброс после успеха) меняет ключ и отменяет начатое подтверждение
  function draftChanged() {
    key.current = crypto.randomUUID()
    setConfirming(false)
  }
  function changeText(next: string) {
    if (next === text) return
    draftChanged()
    setText(next)
  }
  // Загрузка фото заканчивается позже: здесь нельзя трогать текст, замыкание его не видит.
  function changeAudience(next: Audience, nextDays = days) {
    if (next === audience && nextDays === days) return
    draftChanged()
    setAudience(next)
    setDays(nextDays)
  }
  function changePhoto(next?: string, url?: string) {
    draftChanged()
    setPhoto(next)
    setPhotoUrl(url)
  }

  // Лимит — по худшему случаю: у каждого получателя свои значения переменных.
  const len = worstLen(text, globals)
  const limit = photo ? 1024 : 4096
  const over = len - limit
  const hasVars = /\{\{\s*\.[^{}\s]+\s*\}\}/.test(text)
  const blank = visibleText(text).trim() === ''
  const valid = !uploading && over <= 0 && (!!photo || !blank) && (audience !== 'expiring' || daysOk)
  // Для кода после await: файл из уже открытого окна выбора может прийти и под busy.
  const validNow = useRef(valid)
  useEffect(() => {
    validNow.current = valid
  }, [valid])

  // null — грузится или для аудитории нет числа (дни не заданы), undefined — не удалось получить
  const recipients = counts == null ? counts : (counts[audience] ?? null)

  const body = () => ({ text, ...(photo ? { photo } : {}) })
  const broadcastBody = () => ({ ...body(), audience, audienceDays: audience === 'expiring' ? Number(days) : null })

  async function run(fn: () => Promise<void>) {
    if (inFlight.current) return
    inFlight.current = true
    setError('')
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      setError(
        e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось отправить, попробуйте ещё раз',
      )
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  const sendTest = () =>
    run(async () => {
      await post('/api/broadcasts/preview-send', body())
      toast(`Тестовое сообщение отправлено вам, @${OWNER_USERNAME}`, { icon: 'check' })
    })

  function reset() {
    changeText('')
    changePhoto()
  }

  // Диалог называет число получателей на момент клика: на странице оно могло устареть.
  const askSend = () =>
    run(async () => {
      const count = await fetchRecipients(audience, days)
      setCounts(count === undefined ? undefined : (c) => ({ ...c, [audience]: count }))
      if (count && validNow.current) setConfirming(true)
    })

  const broadcast = () =>
    run(async () => {
      setConfirming(false)
      if (!valid) return
      const { broadcast } = await post<{ broadcast: Broadcast }>('/api/broadcasts', { idempotencyKey: key.current, ...broadcastBody() })
      reset()
      if (props.mode === 'broadcast') props.onSent?.(broadcast)
    })

  const direct = () =>
    run(async () => {
      if (props.mode !== 'direct') return
      const b = body()
      const { id } = await post<{ id: number }>(`/api/users/${props.userId}/messages`, b)
      reset()
      props.onSent?.({ id, telegramId: props.userId, ...b, status: 'pending', createdAt: new Date().toISOString() })
    })

  const picker = (
    <PhotoPicker
      value={photo}
      onChange={changePhoto}
      onUploading={(v) => {
        setUploading(v)
        if (v) setConfirming(false)
      }}
      disabled={busy}
    />
  )

  const editor = (
    <TgEditor
      ref={editorRef}
      value={text}
      onChange={changeText}
      disabled={busy}
      label={isBroadcast ? 'Текст рассылки' : 'Текст сообщения'}
      placeholder={isBroadcast ? 'Текст рассылки…' : 'Текст сообщения…'}
      right={
        <span className={cn('tabular-nums', over > 0 && 'text-destructive-strong')}>
          {formatCount(len)} / {formatCount(limit)}
        </span>
      }
      error={
        over <= 0
          ? undefined
          : `${photo ? 'С фото текст становится подписью — не больше 1024 символов' : 'Текст не должен быть длиннее 4096 символов'}${hasVars ? ' (с самыми длинными значениями переменных)' : ''}. Сократите на ${formatCount(over)}.`
      }
      hint="Жирный, курсив, подчёркнутый, зачёркнутый, код в строке, блок кода, спойлер, цитата и ссылки — как в Telegram. Длина считается с самыми длинными значениями переменных."
    />
  )

  const chips = <VarChips title="Переменные — у каждого получателя свои" vars={globals} disabled={busy} onInsert={(name) => editorRef.current?.insert(varLabel(name))} />

  const preview = (
    <div className="flex flex-col overflow-hidden rounded-lg shadow-card">
      <div className={cn('flex items-center gap-2.5 bg-header-bg px-3.5 py-2.5', separator)}>
        <Avatar name={BOT_NAME} colorIndex={5} size={32} />
        <div className="flex flex-col">
          <span className="font-semibold">{BOT_NAME}</span>
          <span className="caption-1 text-text-secondary">бот</span>
        </div>
      </div>
      <div className={cn('flex flex-col justify-end bg-chat-bg px-3 py-4', isBroadcast && 'min-h-90')}>
        <MessageBubble
          time="сейчас"
          media={
            photo &&
            (photoUrl ? (
              <img src={photoUrl} alt="" className="block aspect-video w-full min-w-60 object-cover" />
            ) : (
              <div className="flex aspect-video w-full min-w-60 items-center justify-center bg-fill-tertiary text-text-secondary">
                <Icon name="card" size={32} />
              </div>
            ))
          }
        >
          {blank ? (
            <span className="text-text-hint">{isBroadcast ? 'Текст рассылки появится здесь' : 'Текст сообщения появится здесь'}</span>
          ) : (
            // только разрешённые теги после sanitize
            <span className="whitespace-pre-wrap" dangerouslySetInnerHTML={{ __html: previewHtml(renderPreview(text, Object.fromEntries(globals.map((v) => [v.name, v.sample])))) }} />
          )}
        </MessageBubble>
      </div>
    </div>
  )

  const messages = (
    <>
      {isBroadcast && counts === undefined && (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-destructive-strong">
          Не удалось получить количество получателей
          <Button
            variant="plain"
            size="sm"
            onClick={() => {
              setCounts(null)
              void fetchCounts(daysOk ? days : '').then(setCounts)
            }}
          >
            Повторить
          </Button>
        </div>
      )}
      {error && (
        <p role="alert" className="text-destructive-strong">
          {error}
        </p>
      )}
    </>
  )

  const sendTestButton = (
    <Button variant="gray" icon="send" disabled={busy || !valid} onClick={sendTest}>
      Отправить мне
    </Button>
  )

  if (!isBroadcast) {
    return (
      <div className="admin-body flex flex-col gap-4 text-text">
        {picker}
        {editor}
        {chips}
        {preview}
        {messages}
        <div className="flex flex-wrap justify-end gap-3">
          {sendTestButton}
          <Button loading={busy} disabled={!valid} onClick={direct}>
            Отправить
          </Button>
        </div>
      </div>
    )
  }

  const count = recipients ?? 0

  return (
    <div className="admin-body flex flex-wrap items-start gap-6">
      <div className="flex min-w-0 grow-999 basis-130 flex-col gap-4">
        <Card title="Фото" aside="не больше одного · JPG, PNG или WebP до 10 МБ">
          {picker}
        </Card>

        <Card title={photo ? 'Подпись к фото' : 'Текст сообщения'} aside="Выделите текст и выберите формат">
          {editor}
          {chips}
        </Card>

        <Card title="Кнопки под сообщением" aside="до 3 кнопок · каждая на своей строке">
          <div className="admin-label flex gap-2 px-0.5 text-text-secondary max-md:hidden">
            <span className="flex-[1_1_--spacing(40)]">Текст</span>
            <span className="flex-[1.6_1_--spacing(55)]">Ссылка</span>
            <span className="w-35 flex-none">Стиль</span>
            <span className="w-10 flex-none" />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Input className="min-w-0 flex-[1_1_--spacing(40)]" disabled placeholder="Текст кнопки" aria-label="Текст кнопки" />
            <Input className="min-w-0 flex-[1.6_1_--spacing(55)]" disabled placeholder="https://" aria-label="Ссылка" />
            <div role="radiogroup" aria-label="Стиль кнопки" aria-disabled className="flex w-35 flex-none items-center gap-1.5 opacity-40">
              {BUTTON_STYLES.map((s, i) => (
                <span
                  key={s.label}
                  role="radio"
                  aria-checked={i === 0}
                  aria-disabled
                  aria-label={s.label}
                  title={s.label}
                  className={cn('size-7 rounded-full', s.className)}
                />
              ))}
            </div>
            <Button variant="gray" icon="trash" aria-label="Удалить кнопку" disabled />
          </div>
          <div>
            <Button variant="tinted" icon="plus" disabled title={NOT_WIRED}>
              Добавить кнопку
            </Button>
          </div>
          <p className={note}>{NOT_WIRED}</p>
        </Card>

        <Card title="Получатели" className="gap-2 px-5 pt-5 pb-2">
          <RadioGroup aria-label="Получатели" value={audience} onChange={(v) => changeAudience(v as Audience)} disabled={busy}>
            {AUDIENCES.map(({ value, label }) => (
              <Radio
                key={value}
                value={value}
                label={label}
                className={separator}
                after={
                  <>
                    {value === 'expiring' && (
                      <Input
                        type="number"
                        min={MIN_DAYS}
                        max={MAX_DAYS}
                        inputMode="numeric"
                        aria-label="Дней до окончания подписки"
                        className="w-24"
                        disabled={busy}
                        value={days}
                        error={audience === 'expiring' && !daysOk ? `От ${MIN_DAYS} до ${MAX_DAYS}` : undefined}
                        onFocus={() => changeAudience('expiring')}
                        onChange={(e) => changeAudience('expiring', e.target.value)}
                        after="дн."
                      />
                    )}
                    <span className="min-w-12 text-right text-text-secondary tabular-nums" aria-label={`Получателей: ${label}`}>
                      {counts?.[value] != null ? formatCount(counts[value]) : '—'}
                    </span>
                  </>
                }
              />
            ))}
          </RadioGroup>
          <p className={cn(note, 'pb-2')}>Получатели считаются в момент отправки: не заблокировавшие бота и не заблокированные вами.</p>
        </Card>

        <Card className="gap-0 px-5 py-1">
          <Toggle title="Отправить позже" subtitle="по времени Екатеринбурга (UTC+5)" />
          <Toggle title="Без звука" subtitle="сообщение придёт без уведомления" last />
          <NotWiredHint />
        </Card>

        {messages}

        <div className="flex flex-wrap items-center justify-end gap-3">
          {sendTestButton}
          <Button loading={busy} disabled={!valid || !recipients} onClick={askSend}>
            {recipients == null
              ? 'Отправить'
              : `Отправить ${formatCount(recipients)} ${plural(recipients, ['получателю', 'получателям', 'получателям'])}`}
          </Button>
        </div>
      </div>

      <aside aria-label="Превью сообщения" className="flex max-w-100 min-w-0 grow basis-85 flex-col gap-2 md:sticky md:top-6">
        <h2 className="admin-label px-1 text-text-secondary">Превью с примером данных</h2>
        {preview}
        <p className="caption-1 px-1 text-text-secondary">Приблизительно: у каждого пользователя своя тема и обои чата.</p>
      </aside>

      {confirming && (
        <Dialog
          title="Отправить рассылку?"
          message={`Сообщение сразу получат ${formatCount(count)} ${plural(count, ['пользователь', 'пользователя', 'пользователей'])} из группы «${audienceLabel(audience, days)}». Отменить отправленное нельзя.`}
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setConfirming(false) },
            { label: 'Отправить', onClick: broadcast },
          ]}
        />
      )}
    </div>
  )
}
