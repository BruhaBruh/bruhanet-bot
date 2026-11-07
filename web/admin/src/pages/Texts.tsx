import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { cn } from '@/lib/utils'
import { Button, Card, Dialog, Input, MessageBubble, SearchField, Spinner, StyleSelect, StatusTag, toast } from '@/ui'
import { ApiError, del, get, post, put } from '../api'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { VarChips } from '../editor/VarChips'
import { previewHtml, renderPlain, renderPreview, varLabel, visibleLen } from '../editor/serialize'
import { formatCount } from '../format'
import { previewStyle } from '../lib/button-rows'
import type { KeyboardInfo, KeyboardSlot, TextItem, TextVar } from '../types'
import { KeyboardCard, type SlotView } from './keyboard-editor'
import { SystemButtons } from './system-buttons'
import { sameLayout, type Layout } from './keyboard-layout'

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

export default function Texts() {
  const [params, setParams] = useSearchParams()
  const [items, setItems] = useState<TextItem[]>()
  const [globals, setGlobals] = useState<TextVar[]>([])
  const [loadError, setLoadError] = useState(false)
  const [q, setQ] = useState('')
  // черновики по ключу: переключение между текстами не теряет правки, но и не переносит их
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [styleDrafts, setStyleDrafts] = useState<Record<string, string>>({})
  const [layouts, setLayouts] = useState<Record<string, Layout>>({})
  const [previewKb, setPreviewKb] = useState<string>()
  const [resetKb, setResetKb] = useState<string>()
  const [sysOpen, setSysOpen] = useState(false)
  const [failure, setFailure] = useState<{ key: string; scope: string; message: string }>()
  const [busy, setBusy] = useState<Action>()
  const inFlight = useRef(false)
  const editor = useRef<TgEditorHandle>(null)
  const plain = useRef<HTMLInputElement>(null)

  useEffect(() => {
    get<{ items: TextItem[]; globals: TextVar[] }>('/api/texts').then(
      (r) => {
        setItems(r.items)
        setGlobals(r.globals)
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

  const cur = items.find((i) => i.key === params.get('key')) ?? items[0]
  const isDirty = (i: TextItem) => (drafts[i.key] !== undefined && drafts[i.key] !== i.body) || (styleDrafts[i.key] !== undefined && styleDrafts[i.key] !== i.style)
  const draftOf = (i: TextItem) => drafts[i.key] ?? i.body
  const draft = cur ? draftOf(cur) : ''
  const setDraft = (v: string) => cur && setDrafts((d) => ({ ...d, [cur.key]: v }))
  const dropDraft = (key: string) => {
    setDrafts((d) => {
      const rest = { ...d }
      delete rest[key]
      return rest
    })
    setStyleDrafts((d) => {
      const rest = { ...d }
      delete rest[key]
      return rest
    })
  }
  const refresh = () =>
    get<{ items: TextItem[]; globals: TextVar[] }>('/api/texts').then(
      (r) => {
        setItems(r.items)
        setGlobals(r.globals)
      },
      () => undefined,
    )
  const patch = (key: string, body: string, style?: string) =>
    setItems((list) => list?.map((i) => (i.key === key ? { ...i, body, overridden: body !== i.default, inactive: false, problem: undefined, style: style ?? i.style } : i)))
  const styleOf = (i: TextItem) => styleDrafts[i.key] ?? i.style
  const byKey = (key: string) => items.find((i) => i.key === key)
  const buttonDirty = (key: string) => {
    const b = byKey(key)
    return !!b && (draftOf(b) !== b.body || styleOf(b) !== b.style)
  }
  const keyboards = cur?.kind === 'message' ? (cur.keyboards ?? []) : []
  const layoutOf = (kb: KeyboardInfo): Layout => layouts[kb.id] ?? { rows: kb.rows, hidden: kb.hidden }
  const layoutDirty = (kb: KeyboardInfo) => !sameLayout(layoutOf(kb), kb)
  const buttonKeys = (kb: KeyboardInfo) => kb.slots.filter((x) => x.kind === 'button').map((x) => x.id)
  const kbDirty = (kb: KeyboardInfo) => layoutDirty(kb) || buttonKeys(kb).some(buttonDirty)
  // Кнопки, которых нет ни в одной раскладке (одиночная «Назад», сервисные): правятся окном «Системные кнопки».
  const placed = new Set(items.flatMap((i) => (i.keyboards ?? []).flatMap((kb) => kb.slots.map((x) => x.id))))
  const system = items.filter((i) => i.kind === 'button' && !placed.has(i.key))
  const sampleMap = (it?: TextItem) => Object.fromEntries([...globals, ...(it?.vars ?? [])].map((v) => [v.name, v.sample]))
  const viewOf = (slot: KeyboardSlot): SlotView => {
    const it = byKey(slot.id)
    return it
      ? { title: it.title, label: draftOf(it), style: styleOf(it), route: it.route }
      : { title: slot.id, label: slot.label, style: slot.style ?? '' }
  }
  const shownKb = keyboards.find((k) => k.id === previewKb) ?? keyboards[0]
  const previewRows = shownKb
    ? layoutOf(shownKb).rows.map((row) =>
        row.flatMap((id) => {
          const slot = shownKb.slots.find((x) => x.id === id)
          if (!slot) return []
          if (slot.kind === 'dynamic') return [{ text: slot.label, style: 'default' as const }]
          const v = viewOf(slot)
          return [{ text: renderPlain(v.label, sampleMap(byKey(slot.id))) || slot.id, style: previewStyle(v.style) }]
        }),
      )
    : []

  const s = q.trim().toLowerCase()
  const groups = new Map<string, TextItem[]>()
  for (const i of items) {
    if (i.kind === 'button') continue
    if (s && ![i.title, i.key, draftOf(i)].some((v) => v.toLowerCase().includes(s))) continue
    groups.set(i.group, [...(groups.get(i.group) ?? []), i])
  }

  const own = cur?.vars ?? []
  const vars = [...globals, ...own]
  const samples = Object.fromEntries(vars.map((v) => [v.name, v.sample]))
  const raw = vars.filter((v) => v.raw).map((v) => v.name)
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
      const style = c.kind === 'button' ? styleOf(c) : undefined
      await put(path(c), style === undefined ? { body } : { body, style })
      patch(c.key, body, style)
      dropDraft(c.key)
      toast(c.kind === 'button' ? `Кнопка «${c.title}» сохранена` : `Сообщение «${c.title}» сохранено`, { icon: 'check' })
    })
  // Подписи и стили кнопок — по одной (они общие для кнопки на всех экранах), потом раскладка.
  async function putButtons(keys: string[]) {
    for (const key of keys) {
      const b = byKey(key)
      if (!b || !buttonDirty(key)) continue
      const body = draftOf(b)
      const style = styleOf(b)
      await put(path(b), { body, style })
      patch(key, body, style)
      dropDraft(key)
    }
  }
  const dropLayout = (id: string) =>
    setLayouts((d) => {
      const rest = { ...d }
      delete rest[id]
      return rest
    })
  const saveKeyboard = (kb: KeyboardInfo) =>
    run(
      'save',
      cur,
      async () => {
        try {
          await putButtons(buttonKeys(kb))
          if (layoutDirty(kb)) {
            const l = layoutOf(kb)
            await put(`/api/keyboards/${kb.id}`, { rows: l.rows, hidden: l.hidden })
            dropLayout(kb.id)
          }
          toast(`Клавиатура «${kb.title}» сохранена`, { icon: 'check' })
        } finally {
          await refresh()
        }
      },
      kb.id,
    )
  const resetKeyboard = (kb: KeyboardInfo) =>
    run(
      'reset',
      cur,
      async () => {
        await del(`/api/keyboards/${kb.id}`)
        dropLayout(kb.id)
        toast('Раскладка возвращена как в коде', { icon: 'check' })
        await refresh()
      },
      kb.id,
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
      {sysOpen && <SystemButtons items={system} onClose={() => setSysOpen(false)} onChanged={refresh} />}
      {resetKb && (
        <Dialog
          title="Вернуть клавиатуру как в коде?"
          message="Порядок рядов и скрытые кнопки сбросятся. Подписи и стили кнопок останутся."
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setResetKb(undefined) },
            {
              label: 'Вернуть',
              style: 'destructive',
              onClick: () => {
                const kb = keyboards.find((k) => k.id === resetKb)
                setResetKb(undefined)
                if (kb) void resetKeyboard(kb)
              },
            },
          ]}
        />
      )}
      <nav aria-label="Сообщения бота" className="flex min-w-0 grow basis-70 flex-col gap-4 lg:sticky lg:top-6 lg:max-h-[calc(100dvh-(--spacing(12)))]">
        <Button variant="gray" icon="gear" disabled={!system.length} onClick={() => setSysOpen(true)}>
          Системные кнопки
        </Button>
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
                {cur.kind === 'button' && (
                  <div className="caption-1 flex min-w-0 flex-wrap gap-x-2 font-mono text-text-secondary">
                    <code className="truncate">{cur.key}</code>
                    {cur.route && <code className="truncate">→ {cur.route}</code>}
                  </div>
                )}
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
                {cur.kind === 'button' && <StyleSelect aria-label="Стиль кнопки" disabled={!!busy} value={styleOf(cur)} onChange={(v) => setStyleDrafts((d) => ({ ...d, [cur.key]: v }))} />}
                </div>
                {cur.kind === 'button' && <span className="footnote text-text-secondary">Подпись и стиль общие для всех экранов с этой кнопкой.</span>}
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

          {keyboards.map((kb) => (
            <div key={kb.id} className="flex flex-col gap-2">
              <KeyboardCard
                kb={kb}
                layout={layoutOf(kb)}
                view={viewOf}
                messageTitles={kb.messages.some((m) => m !== cur.key) ? kb.messages.map((m) => byKey(m)?.title ?? m) : []}
                busy={!!busy}
                saving={busy === 'save'}
                dirty={kbDirty(kb)}
                onLayout={(l) => setLayouts((d) => ({ ...d, [kb.id]: l }))}
                onLabel={(id, v) => setDrafts((d) => ({ ...d, [id]: v }))}
                onStyle={(id, v) => setStyleDrafts((d) => ({ ...d, [id]: v }))}
                onSave={() => saveKeyboard(kb)}
                onCancel={() => {
                  dropLayout(kb.id)
                  buttonKeys(kb).forEach(dropDraft)
                  setFailure(undefined)
                }}
                onReset={() => setResetKb(kb.id)}
              />
              {failure?.key === cur.key && failure.scope === kb.id && (
                <p role="alert" className="admin-body text-destructive-strong">
                  {failure.message}
                </p>
              )}
            </div>
          ))}
        </div>
      )}

      {cur && (
        <aside aria-label="Превью сообщения" className="sticky top-6 flex min-w-0 grow basis-80 flex-col gap-2">
          <h2 className="admin-label px-1 text-text-secondary">Превью с примером данных</h2>
          {keyboards.length > 1 && shownKb && (
            <div role="radiogroup" className="flex flex-wrap gap-1.5">
              {keyboards.map((k) => (
                <button
                  key={k.id}
                  type="button"
                  role="radio"
                  aria-checked={k.id === shownKb.id}
                  className={cn('admin-label focus-ring cursor-pointer rounded-md px-2.5 py-1.5 text-left', k.id === shownKb.id ? 'bg-accent-strong text-on-accent-strong' : 'bg-surface text-text')}
                  onClick={() => setPreviewKb(k.id)}
                >
                  {k.title}
                </button>
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
