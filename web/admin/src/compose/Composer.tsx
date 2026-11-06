import { useEffect, useRef, useState } from 'react'
import { BOT_NAME, OWNER_USERNAME } from '@/brand'
import { cn } from '@/lib/utils'
import { Avatar, Button, Card, Dialog, Icon, Input, MessageBubble, Radio, RadioGroup, Switch, toast } from '@/ui'
import { ApiError, get, post, put } from '../api'
import { previewHtml, renderPreview, varLabel, visibleText, worstLen } from '../editor/serialize'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { VarChips } from '../editor/VarChips'
import { formatCount, plural } from '../format'
import type { Audience, Broadcast, BroadcastButton, DirectMessage, TextVar } from '../types'
import { AUDIENCES, audienceLabel, daysValid, DEFAULT_DAYS, MAX_DAYS, MIN_DAYS } from './audience'
import { PhotoPicker } from './PhotoPicker'
import { formatSchedule, isoToMskLocal, mskLocalNow, scheduleInFuture, scheduleShapeOk } from './schedule'

export interface ComposerDraft {
  text: string
  photo?: string
  audience?: Audience
  audienceDays?: number | null
  silent?: boolean
  buttons?: BroadcastButton[]
  scheduledAt?: string | null // ISO
}

// broadcast — всем из панели; direct — одному юзеру (userId)
export type ComposerProps =
  | {
      mode: 'broadcast'
      initial?: ComposerDraft | null
      // открыта существующая рассылка: черновик (автосохранение) или запланированная («Сохранить изменения»)
      broadcastId?: string
      status?: 'draft' | 'scheduled'
      onSent?(b: Broadcast): void
      onSaved?(b: Broadcast): void
    }
  | { mode: 'direct'; userId: number; onSent?(m: DirectMessage): void }

type Counts = Partial<Record<Audience, number>>
// Стили кнопок бота; цвета — как у кнопок в превью.
const BUTTON_STYLES: { value: BroadcastButton['style']; label: string; className: string }[] = [
  { value: '', label: 'Обычная', className: 'bg-keyboard-btn-bg shadow-[inset_0_0_0_1px_var(--separator)]' },
  { value: 'primary', label: 'Основная (синяя)', className: 'bg-accent-strong' },
  { value: 'success', label: 'Успех (зелёная)', className: 'bg-success-strong' },
  { value: 'danger', label: 'Опасная (красная)', className: 'bg-destructive-strong' },
]
const MAX_BUTTONS = 3
const MAX_BUTTON_TEXT = 64
const HTTPS_URL = /^https:\/\/[^\s/?#]+\S*$/i

// Первая проблема в кнопках; warn — просто не заполнено (не ругаем красным).
function buttonsProblem(buttons: BroadcastButton[]): { text: string; warn: boolean } | undefined {
  for (const [i, b] of buttons.entries()) {
    const n = i + 1
    if (b.text.trim() === '' || b.url.trim() === '') return { text: `Кнопка ${n}: заполните текст и ссылку или удалите кнопку`, warn: true }
    if (b.text !== b.text.trim()) return { text: `Кнопка ${n}: уберите пробелы по краям текста`, warn: false }
    if (b.text.includes('{{')) return { text: `Кнопка ${n}: в тексте кнопки нельзя использовать переменные`, warn: false }
    if (!HTTPS_URL.test(b.url)) return { text: `Кнопка ${n}: ссылка должна начинаться с https://`, warn: false }
  }
}

const note = 'footnote text-text-secondary'
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'

function Toggle({ title, subtitle, last, checked, onChange, disabled }: { title: string; subtitle: string; last?: boolean; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <div className={cn('flex min-h-14 items-center gap-3', !last && separator)}>
      <div className="flex grow flex-col">
        <span>{title}</span>
        <span className={note}>{subtitle}</span>
      </div>
      <Switch checked={checked} onChange={onChange} disabled={disabled} aria-label={title} />
    </div>
  )
}

const AUTOSAVE_MS = 1500

const savedTime = new Intl.DateTimeFormat('ru-RU', { timeZone: 'Europe/Moscow', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })

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
  const existing = props.mode === 'broadcast' ? props.broadcastId : undefined
  const scheduledEdit = props.mode === 'broadcast' && props.status === 'scheduled'
  const [text, setText] = useState(initial?.text ?? '')
  const [photo, setPhoto] = useState(initial?.photo)
  const [photoUrl, setPhotoUrl] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [audience, setAudience] = useState<Audience>(initial?.audience ?? 'all')
  const [buttons, setButtons] = useState<BroadcastButton[]>(initial?.buttons ?? [])
  const [silent, setSilent] = useState(initial?.silent ?? false)
  const [later, setLater] = useState(scheduledEdit || !!initial?.scheduledAt)
  const [at, setAt] = useState(initial?.scheduledAt ? isoToMskLocal(initial.scheduledAt) : '')
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
  const atOk = !later || scheduleInFuture(at)
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
  function changeButtons(next: BroadcastButton[]) {
    draftChanged()
    setButtons(next)
  }
  function changeButton(i: number, patch: Partial<BroadcastButton>) {
    changeButtons(buttons.map((b, j) => (j === i ? { ...b, ...patch } : b)))
  }
  function changeSilent(next: boolean) {
    draftChanged()
    setSilent(next)
  }
  function changeLater(next: boolean) {
    draftChanged()
    setLater(next)
  }
  function changeAt(next: string) {
    draftChanged()
    setAt(next)
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
  const buttonsIssue = isBroadcast ? buttonsProblem(buttons) : undefined
  const valid = !uploading && over <= 0 && (!!photo || !blank) && (audience !== 'expiring' || daysOk) && !buttonsIssue && (!isBroadcast || atOk) && (!scheduledEdit || (later && at !== ''))
  // Для кода после await: файл из уже открытого окна выбора может прийти и под busy.
  const validNow = useRef(valid)
  useEffect(() => {
    validNow.current = valid
  }, [valid])

  // null — грузится или для аудитории нет числа (дни не заданы), undefined — не удалось получить
  const recipients = counts == null ? counts : (counts[audience] ?? null)

  const body = () => ({ text, ...(photo ? { photo } : {}) })
  const broadcastBody = () => ({
    ...body(),
    audience,
    audienceDays: audience === 'expiring' && daysOk ? Number(days) : null,
    silent,
    buttons,
    scheduledAt: later ? at : '',
  })

  // Автосохранение черновика: PUT с id, выбранным клиентом (повторы и гонки не плодят строки). Запросы идут по цепочке,
  // чтобы порядок сохранений совпадал с порядком правок. Открытый пустой редактор черновика не создаёт — только правка.
  const draftId = useRef(existing ?? crypto.randomUUID())
  const draftExists = useRef(!!existing)
  const chain = useRef<Promise<void>>(Promise.resolve())
  const finished = useRef(false) // отправлено: черновика больше нет
  const [savedAt, setSavedAt] = useState<Date | null>(null)
  const [saveFailed, setSaveFailed] = useState(false)
  const draftPayload = () => {
    const b = broadcastBody()
    return { ...b, scheduledAt: later && scheduleShapeOk(at) ? at : '' }
  }
  const snapshot = isBroadcast && !scheduledEdit ? JSON.stringify(draftPayload()) : ''
  const lastSaved = useRef(snapshot)
  const pending = useRef<string>('')
  const saveNow = useRef<() => void>(() => {})
  const save = () => {
    const snap = pending.current
    if (!snap || snap === lastSaved.current || finished.current) return
    pending.current = ''
    const next = chain.current.then(async () => {
      if (finished.current) return
      try {
        await put(`/api/broadcasts/${draftId.current}`, JSON.parse(snap))
        draftExists.current = true
        lastSaved.current = snap
        setSavedAt(new Date())
        setSaveFailed(false)
      } catch {
        setSaveFailed(true)
      }
    })
    chain.current = next
  }
  useEffect(() => {
    saveNow.current = save
  })
  useEffect(() => {
    if (!snapshot || snapshot === lastSaved.current) return
    pending.current = snapshot
    const timer = setTimeout(() => saveNow.current(), AUTOSAVE_MS)
    return () => clearTimeout(timer)
  }, [snapshot])
  // Ушли со страницы в пределах задержки — не теряем последние правки.
  useEffect(
    () => () => {
      saveNow.current()
    },
    [],
  )

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
      await post('/api/broadcasts/preview-send', isBroadcast ? { ...body(), silent, buttons } : body())
      toast(`Тестовое сообщение отправлено вам, @${OWNER_USERNAME}`, { icon: 'check' })
    })

  function reset() {
    changeText('')
    changePhoto()
    changeButtons([])
    changeSilent(false)
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
      // Отправка не должна гоняться с автосохранением: дожидаемся уже начатого и гасим отложенное.
      pending.current = ''
      await chain.current
      const { broadcast } = await post<{ broadcast: Broadcast }>('/api/broadcasts', {
        idempotencyKey: key.current,
        ...(draftExists.current ? { draftId: draftId.current } : {}),
        ...broadcastBody(),
        scheduledAt: later ? at : undefined,
      })
      finished.current = true
      reset()
      if (props.mode === 'broadcast') props.onSent?.(broadcast)
    })

  // Правка запланированной рассылки: до старта, время обязательно.
  const saveScheduled = () =>
    run(async () => {
      const { broadcast } = await put<{ broadcast: Broadcast }>(`/api/broadcasts/${existing}`, { ...broadcastBody(), scheduledAt: at })
      toast('Изменения сохранены', { icon: 'check' })
      if (props.mode === 'broadcast') props.onSaved?.(broadcast)
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
          keyboard={buttons
            .filter((b) => b.text.trim() !== '')
            .map((b) => [{ text: b.text, url: b.url || undefined, style: b.style || 'default' }])}
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
      {isBroadcast && !scheduledEdit && (
        <p className={cn(note, saveFailed && 'text-destructive-strong')} role={saveFailed ? 'alert' : undefined}>
          {saveFailed
            ? 'Не удалось сохранить черновик — проверьте соединение, правки сохранятся при следующем изменении'
            : savedAt
              ? `Черновик сохранён · ${savedTime.format(savedAt)}`
              : 'Черновик сохраняется автоматически'}
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
          {buttons.length > 0 && (
            <div className="admin-label flex gap-2 px-0.5 text-text-secondary max-md:hidden">
              <span className="flex-[1_1_--spacing(40)]">Текст</span>
              <span className="flex-[1.6_1_--spacing(55)]">Ссылка</span>
              <span className="w-35 flex-none">Стиль</span>
              <span className="w-10 flex-none" />
            </div>
          )}
          {buttons.map((b, i) => (
            <div key={i} className="flex flex-wrap items-center gap-2">
              <Input
                className="min-w-0 flex-[1_1_--spacing(40)]"
                placeholder="Текст кнопки"
                aria-label={`Текст кнопки ${i + 1}`}
                maxLength={MAX_BUTTON_TEXT}
                disabled={busy}
                value={b.text}
                onChange={(e) => changeButton(i, { text: e.target.value })}
              />
              <Input
                className="min-w-0 flex-[1.6_1_--spacing(55)]"
                placeholder="https://"
                aria-label={`Ссылка кнопки ${i + 1}`}
                inputMode="url"
                disabled={busy}
                value={b.url}
                onChange={(e) => changeButton(i, { url: e.target.value })}
              />
              <div role="radiogroup" aria-label={`Стиль кнопки ${i + 1}`} className="flex w-35 flex-none items-center gap-1.5">
                {BUTTON_STYLES.map((st) => (
                  <button
                    key={st.value}
                    type="button"
                    role="radio"
                    aria-checked={b.style === st.value}
                    aria-label={st.label}
                    title={st.label}
                    disabled={busy}
                    onClick={() => changeButton(i, { style: st.value })}
                    className={cn(
                      'focus-ring size-7 cursor-pointer rounded-full disabled:cursor-default disabled:opacity-40',
                      st.className,
                      b.style === st.value && 'outline-2 outline-offset-2 outline-accent',
                    )}
                  />
                ))}
              </div>
              <Button variant="gray" icon="trash" aria-label={`Удалить кнопку ${i + 1}`} disabled={busy} onClick={() => changeButtons(buttons.filter((_, j) => j !== i))} />
            </div>
          ))}
          {buttonsIssue && (
            <p role="alert" className={buttonsIssue.warn ? note : cn(note, 'text-destructive-strong')}>
              {buttonsIssue.text}
            </p>
          )}
          <div>
            <Button
              variant="tinted"
              icon="plus"
              disabled={busy || buttons.length >= MAX_BUTTONS}
              onClick={() => changeButtons([...buttons, { text: '', url: '', style: '' }])}
            >
              Добавить кнопку
            </Button>
          </div>
          <p className={note}>Только ссылки https://. Текст кнопки — простой текст до 64 символов, без переменных.</p>
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
          <Toggle
            title="Отправить позже"
            subtitle="по московскому времени (UTC+3)"
            checked={later}
            onChange={changeLater}
            disabled={busy || scheduledEdit}
          />
          {later && (
            <div className={cn('py-3', separator)}>
              <Input
                type="datetime-local"
                aria-label="Дата и время отправки"
                min={mskLocalNow()}
                step={60}
                disabled={busy}
                value={at}
                error={at !== '' && !atOk ? 'Это время уже прошло — выберите будущее' : undefined}
                hint={at === '' ? 'Время по Москве' : undefined}
                onChange={(e) => changeAt(e.target.value)}
              />
            </div>
          )}
          <Toggle title="Без звука" subtitle="сообщение придёт без уведомления" last checked={silent} onChange={changeSilent} disabled={busy} />
        </Card>

        {messages}

        <div className="flex flex-wrap items-center justify-end gap-3">
          {sendTestButton}
          {scheduledEdit ? (
            <Button loading={busy} disabled={!valid} onClick={saveScheduled}>
              Сохранить изменения
            </Button>
          ) : (
            <Button loading={busy} disabled={!valid || !recipients} onClick={askSend}>
              {later
                ? 'Запланировать'
                : recipients == null
                  ? 'Отправить'
                  : `Отправить ${formatCount(recipients)} ${plural(recipients, ['получателю', 'получателям', 'получателям'])}`}
            </Button>
          )}
        </div>
      </div>

      <aside aria-label="Превью сообщения" className="flex max-w-100 min-w-0 grow basis-85 flex-col gap-2 md:sticky md:top-6">
        <h2 className="admin-label px-1 text-text-secondary">Превью с примером данных</h2>
        {preview}
        <p className="caption-1 px-1 text-text-secondary">Приблизительно: у каждого пользователя своя тема и обои чата.</p>
      </aside>

      {confirming && (
        <Dialog
          title={later ? 'Запланировать рассылку?' : 'Отправить рассылку?'}
          message={
            later
              ? `Сообщение получат ${formatCount(count)} ${plural(count, ['пользователь', 'пользователя', 'пользователей'])} из группы «${audienceLabel(audience, days)}» — ${formatSchedule(at)} по Москве. ${silent ? 'Придёт без звука. ' : ''}Получатели пересчитаются в момент отправки. До отправки рассылку можно изменить или отменить.`
              : `Сообщение сразу получат ${formatCount(count)} ${plural(count, ['пользователь', 'пользователя', 'пользователей'])} из группы «${audienceLabel(audience, days)}». ${silent ? 'Придёт без звука. ' : ''}Отменить отправленное нельзя.`
          }
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setConfirming(false) },
            { label: later ? 'Запланировать' : 'Отправить', onClick: broadcast },
          ]}
        />
      )}
    </div>
  )
}
