import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { cn } from '@/lib/utils'
import { Button, Card, Checkbox, Dialog, Input, MessageBubble, SearchField, Spinner, StatusTag, toast } from '@/ui'
import { ApiError, del, get, post, put } from '../api'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { VarChips } from '../editor/VarChips'
import { previewHtml, renderPlain, renderPreview, varLabel, visibleLen } from '../editor/serialize'
import { formatCount } from '../format'
import { previewStyle } from '../lib/button-rows'
import { payload, sameKb, usedConditions, visibleRows } from '../lib/message-keyboard'
import type { KbCatalogs, KbCellOut, TextItem, TextVar } from '../types'
import { MessageKeyboardCard } from './message-keyboard'

interface TextsReply {
  items: TextItem[]
  globals: TextVar[]
  conditions?: KbCatalogs['conditions']
  actions?: KbCatalogs['actions']
  templates?: KbCatalogs['templates']
}

const path = (c: TextItem) => `/api/texts/${encodeURIComponent(c.key)}`

const MESSAGE_LIMIT = 4096
const BUTTON_LIMIT = 64
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'
const counter = (n: number, limit: number) => (
  <span className={cn('caption-1 tabular-nums', n > limit && 'text-destructive-strong')}>
    {formatCount(n)} / {formatCount(limit)}
  </span>
)

type Action = 'save' | 'test' | 'reset'
const TEXT_SCOPE = 'text'
const KB_SCOPE = 'keyboard'

export default function Texts() {
  const [params, setParams] = useSearchParams()
  const [items, setItems] = useState<TextItem[]>()
  const [globals, setGlobals] = useState<TextVar[]>([])
  const [loadError, setLoadError] = useState(false)
  const [q, setQ] = useState('')
  // черновики по ключу: переключение между текстами не теряет правки, но и не переносит их
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [cat, setCat] = useState<KbCatalogs>({ conditions: [], actions: [], templates: [] })
  // черновики клавиатур по ключу сообщения
  const [kbDrafts, setKbDrafts] = useState<Record<string, KbCellOut[][]>>({})
  // «фейк-условия» превью: какие условия считать выполненными
  const [fake, setFake] = useState<string[]>([])
  const [resetKb, setResetKb] = useState(false)
  const [failure, setFailure] = useState<{ key: string; scope: string; message: string }>()
  const [busy, setBusy] = useState<Action>()
  const inFlight = useRef(false)
  const editor = useRef<TgEditorHandle>(null)
  const plain = useRef<HTMLInputElement>(null)

  useEffect(() => {
    get<TextsReply>('/api/texts').then(
      (r) => {
        setItems(r.items)
        setGlobals(r.globals)
        setCat({ conditions: r.conditions ?? [], actions: r.actions ?? [], templates: r.templates ?? [] })
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
  if (!items) return <Spinner className="self-center p-8" />

  // кнопки реестра в редакторе не показываются: подпись и стиль задаются в клавиатуре сообщения
  const editable = items.filter((i) => i.kind !== 'button')
  const cur = editable.find((i) => i.key === params.get('key')) ?? editable[0]
  const isDirty = (i: TextItem) => drafts[i.key] !== undefined && drafts[i.key] !== i.body
  const draftOf = (i: TextItem) => drafts[i.key] ?? i.body
  const draft = cur ? draftOf(cur) : ''
  const setDraft = (v: string) => cur && setDrafts((d) => ({ ...d, [cur.key]: v }))
  const dropDraft = (key: string) => {
    setDrafts((d) => {
      const rest = { ...d }
      delete rest[key]
      return rest
    })
  }
  const refresh = () =>
    get<TextsReply>('/api/texts').then(
      (r) => {
        setItems(r.items)
        setGlobals(r.globals)
        setCat({ conditions: r.conditions ?? [], actions: r.actions ?? [], templates: r.templates ?? [] })
      },
      () => undefined,
    )
  const patch = (key: string, body: string) => setItems((list) => list?.map((i) => (i.key === key ? { ...i, body, overridden: body !== i.default, inactive: false, problem: undefined } : i)))

  const own = cur?.vars ?? []
  const vars = [...globals, ...own]
  const samples = Object.fromEntries(vars.map((v) => [v.name, v.sample]))
  const raw = vars.filter((v) => v.raw).map((v) => v.name)

  const kb = cur?.kind === 'message' ? cur.keyboard : undefined
  const kbRows = kb ? (kbDrafts[cur.key] ?? kb.rows) : []
  const kbDirty = !!kb && !sameKb(kbRows, kb.rows)
  const dropKbDraft = (key: string) =>
    setKbDrafts((d) => {
      const rest = { ...d }
      delete rest[key]
      return rest
    })
  const condIds = kb ? usedConditions(kbRows, cat.actions) : []
  const fakeSet = new Set(fake)
  const previewRows = visibleRows(kbRows, fakeSet, cat.actions).map((row) =>
    row.map((c) => ({
      text: c.template
        ? (cat.templates.find((t) => `@${t.id}` === c.id)?.title ?? c.id)
        : (c.label ? renderPlain(c.label, samples) : c.effective?.preview) || c.id,
      style: previewStyle(c.template ? '' : c.style || c.effective?.style || ''),
    })),
  )

  const s = q.trim().toLowerCase()
  const groups = new Map<string, TextItem[]>()
  for (const i of editable) {
    if (s && ![i.title, i.key, draftOf(i)].some((v) => v.toLowerCase().includes(s))) continue
    groups.set(i.group, [...(groups.get(i.group) ?? []), i])
  }

  const used = vars.filter((v) => draft.includes(varLabel(v.name)))
  const shownVars = used.length ? used : own.slice(0, 3)

  function insertVar(name: string) {
    const text = varLabel(name)
    if (cur?.kind === 'message') return editor.current?.insert(text)
    const at = plain.current?.selectionStart ?? draft.length
    const end = plain.current?.selectionEnd ?? at
    setDraft(draft.slice(0, at) + text + draft.slice(end))
  }

  // один запрос за раз: двойной клик не шлёт второй
  async function run(action: Action, c: TextItem, fn: () => Promise<void>, scope = TEXT_SCOPE) {
    if (inFlight.current) return
    inFlight.current = true
    setFailure(undefined)
    setBusy(action)
    try {
      await fn()
    } catch (e) {
      setFailure({
        key: c.key,
        scope,
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
  const saveKeyboard = (c: TextItem) =>
    run(
      'save',
      c,
      async () => {
        await put(`${path(c)}/keyboard`, payload(kbRows))
        dropKbDraft(c.key)
        toast('Клавиатура сохранена', { icon: 'check' })
        await refresh()
      },
      KB_SCOPE,
    )
  const resetKeyboard = (c: TextItem) =>
    run(
      'reset',
      c,
      async () => {
        await del(`${path(c)}/keyboard`)
        dropKbDraft(c.key)
        toast('Клавиатура возвращена как в коде', { icon: 'check' })
        await refresh()
      },
      KB_SCOPE,
    )
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
      {resetKb && cur && (
        <Dialog
          title="Вернуть клавиатуру как в коде?"
          message="Все изменения клавиатуры этого сообщения пропадут."
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setResetKb(false) },
            {
              label: 'Вернуть',
              style: 'destructive',
              onClick: () => {
                setResetKb(false)
                void resetKeyboard(cur)
              },
            },
          ]}
        />
      )}
      <nav aria-label="Сообщения бота" className="flex min-w-0 grow basis-70 flex-col gap-4 lg:sticky lg:top-6 lg:max-h-[calc(100dvh-7.5rem)]">
        <SearchField placeholder="Найти сообщение" value={q} onChange={(e) => setQ(e.target.value)} onClear={() => setQ('')} />
        {/* Длинный список прокручивается сам: не отодвигает редактор и на компьютере закреплён рядом с ним. */}
        <div className="flex min-h-0 flex-col gap-4 overflow-y-auto max-lg:max-h-72">
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
                        on ? 'bg-accent-strong text-on-accent-strong' : 'bg-surface text-text hover:bg-cell-highlight',
                      )}
                      onClick={() => setParams({ key: i.key })}
                    >
                      <span className="flex min-w-0 grow flex-col">
                        <span className="font-medium">{i.title}</span>
                        <span className={cn('caption-1 truncate font-mono', !on && 'opacity-75')}>{i.key}</span>
                      </span>
                      {i.inactive && <StatusTag tone="destructive">не применяется</StatusTag>}
                      {(i.overridden || dirty) && (
                        <span
                          role="img"
                          aria-label={dirty ? 'Есть несохранённые изменения' : 'Изменено'}
                          title={dirty ? 'Есть несохранённые изменения' : 'Изменено'}
                          className={cn('size-2 flex-none rounded-full', on ? 'bg-on-accent-strong' : 'bg-warning')}
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
                <span className="footnote text-text-secondary">{cur.when}</span>
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
                hint="Переменные подставляются при отправке. Неизвестная переменная не сохранится."
              />
            ) : (
              <div className="flex flex-col gap-2">
                <div className="flex items-start gap-2">
                <Input
                  className="min-w-0 grow"
                  ref={plain}
                  aria-label={cur.title}
                  placeholder={cur.title}
                  maxLength={BUTTON_LIMIT}
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  after={counter(draft.length, BUTTON_LIMIT)}
                  hint={`Простой текст без разметки, до ${BUTTON_LIMIT} символов.`}
                />
                </div>
              </div>
            )}

            <VarChips title="Общие — нажмите, чтобы вставить" vars={globals} disabled={!!busy} onInsert={insertVar} />
            <VarChips title="Этого сообщения" vars={own} disabled={!!busy} onInsert={insertVar} />
          </Card>

          {cur.inactive && (
            <p className="admin-body rounded-lg bg-destructive-tint px-4 py-3 text-destructive-strong">
              Сохранённый текст не применяется, пользователи видят стандартный: {cur.problem}
            </p>
          )}

          {failure?.key === cur.key && failure.scope === TEXT_SCOPE && (
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

          {kb && (
            <div className="flex flex-col gap-2">
              <MessageKeyboardCard
                kb={kb}
                cat={cat}
                rows={kbRows}
                busy={!!busy}
                saving={busy === 'save'}
                dirty={kbDirty}
                onRows={(r) => setKbDrafts((d) => ({ ...d, [cur.key]: r }))}
                onSave={() => saveKeyboard(cur)}
                onCancel={() => {
                  dropKbDraft(cur.key)
                  setFailure(undefined)
                }}
                onReset={() => setResetKb(true)}
              />
              {failure?.key === cur.key && failure.scope === KB_SCOPE && (
                <p role="alert" className="admin-body text-destructive-strong">
                  {failure.message}
                </p>
              )}
            </div>
          )}
        </div>
      )}

      {cur && (
        <aside aria-label="Превью сообщения" className="sticky top-6 flex min-w-0 grow basis-80 flex-col gap-2">
          <h2 className="admin-label px-1 text-text-secondary">Превью с примером данных</h2>
          {kb && condIds.length > 0 && (
            <div role="group" aria-label="Условия для превью" className="flex flex-col gap-2 rounded-lg bg-surface px-4 py-3 shadow-card">
              <span className="admin-label text-text-secondary">Условия для превью</span>
              {condIds.map((id) => (
                <Checkbox
                  key={id}
                  checked={fakeSet.has(id)}
                  label={cat.conditions.find((c) => c.id === id)?.title ?? id}
                  onChange={(on) => setFake((f) => (on ? [...f, id] : f.filter((x) => x !== id)))}
                />
              ))}
            </div>
          )}
          <div className="flex min-h-80 flex-col justify-end overflow-hidden rounded-lg bg-chat-bg px-3 py-4 shadow-card">
            <MessageBubble time="14:30" keyboard={previewRows}>
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
