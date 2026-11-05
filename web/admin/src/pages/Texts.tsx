import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { NOT_WIRED } from '@/lib/not-wired'
import { cn } from '@/lib/utils'
import { Button, Card, Input, MessageBubble, SearchField, Spinner, toast } from '@/ui'
import { ApiError, del, get, post, put } from '../api'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { previewHtml, renderPlain, renderPreview, visibleLen } from '../editor/serialize'
import { formatCount } from '../format'
import type { TextItem } from '../types'

const varLabel = (name: string) => `{{.${name}}}`
const path = (c: TextItem) => `/api/texts/${encodeURIComponent(c.key)}`

const MESSAGE_LIMIT = 4096
const BUTTON_LIMIT = 64
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'
const counter = (n: number, limit: number) => (
  <span className={cn('caption-1 tabular-nums', n > limit && 'text-destructive-strong')}>
    {formatCount(n)} / {formatCount(limit)}
  </span>
)

const SWATCHES = [
  { label: 'Обычная', className: 'bg-keyboard-btn-bg shadow-[0_0_0_2px_var(--surface),0_0_0_4px_var(--accent),inset_0_0_0_1px_var(--separator)]' },
  { label: 'Основная (синяя)', className: 'bg-accent-strong' },
  { label: 'Успех (зелёная)', className: 'bg-success-strong' },
  { label: 'Опасная (красная)', className: 'bg-destructive-strong' },
]

type Action = 'save' | 'test' | 'reset'

export default function Texts() {
  const [params, setParams] = useSearchParams()
  const [items, setItems] = useState<TextItem[]>()
  const [loadError, setLoadError] = useState(false)
  const [q, setQ] = useState('')
  // черновики по ключу: переключение между текстами не теряет правки, но и не переносит их
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [failure, setFailure] = useState<{ key: string; message: string }>()
  const [busy, setBusy] = useState<Action>()
  const inFlight = useRef(false)
  const editor = useRef<TgEditorHandle>(null)
  const plain = useRef<HTMLInputElement>(null)

  useEffect(() => {
    get<{ items: TextItem[] }>('/api/texts').then(
      (r) => setItems(r.items),
      () => setLoadError(true),
    )
  }, [])

  if (loadError)
    return (
      <p role="alert" className="admin-body text-destructive-strong">
        Не удалось загрузить
      </p>
    )
  if (!items) return <Spinner className="self-center p-8" />

  const cur = items.find((i) => i.key === params.get('key')) ?? items[0]
  const isDirty = (i: TextItem) => drafts[i.key] !== undefined && drafts[i.key] !== i.body
  const draftOf = (i: TextItem) => drafts[i.key] ?? i.body
  const draft = cur ? draftOf(cur) : ''
  const setDraft = (v: string) => cur && setDrafts((d) => ({ ...d, [cur.key]: v }))
  const dropDraft = (key: string) =>
    setDrafts((d) => {
      const rest = { ...d }
      delete rest[key]
      return rest
    })
  const patch = (key: string, body: string) =>
    setItems((list) => list?.map((i) => (i.key === key ? { ...i, body, overridden: body !== i.default } : i)))

  const s = q.trim().toLowerCase()
  const groups = new Map<string, TextItem[]>()
  for (const i of items) {
    if (s && ![i.title, i.key, draftOf(i)].some((v) => v.toLowerCase().includes(s))) continue
    groups.set(i.group, [...(groups.get(i.group) ?? []), i])
  }

  const vars = cur?.vars ?? []
  const samples = Object.fromEntries(vars.map((v) => [v.name, v.sample]))
  const raw = vars.filter((v) => v.raw).map((v) => v.name)
  const used = vars.filter((v) => draft.includes(varLabel(v.name)))
  const shownVars = used.length ? used : vars.slice(0, 3)

  function insertVar(name: string) {
    const text = varLabel(name)
    if (cur?.kind === 'message') return editor.current?.insert(text)
    const at = plain.current?.selectionStart ?? draft.length
    const end = plain.current?.selectionEnd ?? at
    setDraft(draft.slice(0, at) + text + draft.slice(end))
  }

  // один запрос за раз: двойной клик не шлёт второй
  async function run(action: Action, c: TextItem, fn: () => Promise<void>) {
    if (inFlight.current) return
    inFlight.current = true
    setFailure(undefined)
    setBusy(action)
    try {
      await fn()
    } catch (e) {
      setFailure({
        key: c.key,
        message: e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось выполнить, попробуйте ещё раз',
      })
    } finally {
      inFlight.current = false
      setBusy(undefined)
    }
  }

  const save = (c: TextItem, body: string) =>
    run('save', c, async () => {
      await put(path(c), { body })
      patch(c.key, body)
      dropDraft(c.key)
      toast(`Сообщение «${c.title}» сохранено`, { icon: 'check' })
    })
  const sendTest = (c: TextItem, body: string) =>
    run('test', c, async () => {
      await post(`${path(c)}/test`, { body })
      toast('Сообщение отправлено вам в Telegram', { icon: 'check' })
    })
  const reset = (c: TextItem) =>
    run('reset', c, async () => {
      await del(path(c))
      patch(c.key, c.default)
      dropDraft(c.key)
      toast('Стандартный текст возвращён', { icon: 'check' })
    })

  return (
    <div className="flex flex-wrap items-start gap-6">
      <nav aria-label="Сообщения бота" className="flex min-w-0 grow basis-70 flex-col gap-4">
        <SearchField placeholder="Найти сообщение" value={q} onChange={(e) => setQ(e.target.value)} onClear={() => setQ('')} />
        {/* Когда колонки сложены, длинный список не отодвигает редактор на несколько экранов. */}
        <div className="flex flex-col gap-4 max-lg:max-h-72 max-lg:overflow-y-auto">
          {!groups.size && <p className="admin-body px-1 text-text-secondary">Ничего не найдено</p>}
          {[...groups].map(([group, list]) => (
            <div key={group} className="flex flex-col gap-1.5">
              <span className="admin-label px-1 text-text-secondary">{group}</span>
              <div className="flex flex-col overflow-hidden rounded-lg bg-surface shadow-card">
                {list.map((i) => {
                  const on = i.key === cur?.key
                  const dirty = isDirty(i)
                  return (
                    <button
                      key={i.key}
                      type="button"
                      aria-current={on ? 'page' : undefined}
                      className={cn(
                        'admin-body focus-ring flex cursor-pointer items-center gap-2.5 px-3.5 py-2.5 text-left focus-visible:-outline-offset-2',
                        separator,
                        on ? 'bg-accent text-on-accent' : 'bg-surface text-text hover:bg-cell-highlight',
                      )}
                      onClick={() => setParams({ key: i.key })}
                    >
                      <span className="flex min-w-0 grow flex-col">
                        <span className="font-medium">{i.title}</span>
                        <span className="caption-1 truncate font-mono opacity-75">{i.key}</span>
                      </span>
                      {(i.overridden || dirty) && (
                        <span
                          role="img"
                          aria-label={dirty ? 'Есть несохранённые изменения' : 'Изменено'}
                          title={dirty ? 'Есть несохранённые изменения' : 'Изменено'}
                          className={cn('size-2 flex-none rounded-full', on ? 'bg-on-accent' : 'bg-warning')}
                        />
                      )}
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </nav>

      {cur && (
        <div className="flex min-w-0 grow-999 basis-110 flex-col gap-4">
          <Card>
            <div className="flex flex-wrap items-start gap-3">
              <div className="flex min-w-0 grow flex-col gap-0.5">
                <h2 className="title-3 text-text">{cur.title}</h2>
                <span className="footnote text-text-secondary">—</span>
              </div>
              <code className="caption-1 max-w-full truncate rounded-xs bg-fill-tertiary px-2 py-1 font-mono text-text-secondary">{cur.key}</code>
            </div>

            {cur.kind === 'message' ? (
              <TgEditor
                key={cur.key}
                ref={editor}
                value={draft}
                onChange={setDraft}
                disabled={!!busy}
                right={counter(visibleLen(draft), MESSAGE_LIMIT)}
                hint="Переменные подставляются при отправке. Неизвестная переменная останется как есть — она подсвечена в превью."
              />
            ) : (
              <Input
                ref={plain}
                aria-label={cur.title}
                placeholder={cur.title}
                maxLength={BUTTON_LIMIT}
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                after={counter(draft.length, BUTTON_LIMIT)}
                hint={`Простой текст без разметки, до ${BUTTON_LIMIT} символов.`}
              />
            )}

            {vars.length > 0 && (
              <div className="flex flex-col gap-2">
                <span className="admin-label text-text-secondary">Переменные — нажмите, чтобы вставить</span>
                <div className="flex flex-wrap gap-1.5">
                  {vars.map((v) => (
                    <button
                      key={v.name}
                      type="button"
                      title={`${v.description} — например, ${v.sample}`}
                      disabled={!!busy}
                      className="mono focus-ring inline-flex h-7 max-w-full cursor-pointer items-center rounded-full bg-accent-tint px-2.5 font-semibold text-accent-strong disabled:cursor-default disabled:opacity-40"
                      onClick={() => insertVar(v.name)}
                    >
                      <span className="truncate">{varLabel(v.name)}</span>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </Card>

          <Card title="Кнопки" aside={NOT_WIRED} className="gap-2.5">
            <div className="flex flex-wrap items-center gap-2">
              <Input className="min-w-0 grow basis-50" disabled placeholder="Текст кнопки" aria-label="Текст кнопки" value="" readOnly />
              <code className="caption-1 min-w-0 shrink basis-50 truncate font-mono text-text-secondary">—</code>
              <div role="radiogroup" aria-label="Стиль кнопки" className="flex w-35 flex-none items-center gap-1.5">
                {SWATCHES.map((sw, n) => (
                  <button
                    key={sw.label}
                    type="button"
                    role="radio"
                    aria-checked={n === 0}
                    aria-label={sw.label}
                    title={sw.label}
                    disabled
                    className={cn('size-7 cursor-default rounded-full opacity-40', sw.className)}
                  />
                ))}
              </div>
            </div>
            <span className="footnote text-text-secondary">Действие кнопки задаётся в коде бота; здесь меняется только подпись и стиль.</span>
          </Card>

          {failure?.key === cur.key && (
            <p role="alert" className="admin-body text-destructive-strong">
              {failure.message}
            </p>
          )}

          <div className="flex flex-wrap justify-end gap-3">
            {isDirty(cur) && (
              <Button
                variant="plain"
                disabled={!!busy}
                onClick={() => {
                  dropDraft(cur.key)
                  setFailure(undefined)
                }}
              >
                Отменить
              </Button>
            )}
            <Button variant="gray" icon="refresh" loading={busy === 'reset'} disabled={!!busy || !cur.overridden} onClick={() => reset(cur)}>
              Вернуть стандартный текст
            </Button>
            <Button variant="gray" icon="send" loading={busy === 'test'} disabled={!!busy} onClick={() => sendTest(cur, draft)}>
              Отправить себе
            </Button>
            <Button loading={busy === 'save'} disabled={!!busy || !isDirty(cur)} onClick={() => save(cur, draft)}>
              Сохранить
            </Button>
          </div>
        </div>
      )}

      {cur && (
        <aside aria-label="Превью сообщения" className="sticky top-6 flex min-w-0 grow basis-80 flex-col gap-2">
          <h2 className="admin-label px-1 text-text-secondary">Превью с примером данных</h2>
          <div className="flex min-h-80 flex-col justify-end overflow-hidden rounded-lg bg-chat-bg px-3 py-4 shadow-card">
            <MessageBubble time="14:30">
              {cur.kind === 'message' ? (
                // только разрешённые теги после sanitize
                <span className="whitespace-pre-wrap" dangerouslySetInnerHTML={{ __html: previewHtml(renderPreview(draft, samples, raw)) }} />
              ) : (
                <span className="whitespace-pre-wrap">{renderPlain(draft, samples)}</span>
              )}
            </MessageBubble>
          </div>
          {shownVars.length > 0 && (
            <div className="rounded-lg bg-surface px-4 py-2 shadow-card">
              {shownVars.map((v) => (
                <div key={v.name} className={cn('footnote flex justify-between gap-3 py-1.5', separator)}>
                  <code className="min-w-0 font-mono break-all text-accent-strong">{varLabel(v.name)}</code>
                  <span className="min-w-0 text-right wrap-anywhere text-text-secondary">{v.sample}</span>
                </div>
              ))}
            </div>
          )}
        </aside>
      )}
    </div>
  )
}
