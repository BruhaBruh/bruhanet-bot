import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { cn } from '@/lib/utils'
import { Button, Card, Checkbox, Dialog, Icon, Input, MessageBubble, SearchField, SegmentedControl, Spinner, Tip, StatusTag, toast } from '@/ui'
import { ApiError, del, get, post, put } from '../api'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { VarChips } from '../editor/VarChips'
import { previewHtml, renderPlain, renderPreview, varLabel, visibleLen } from '../editor/serialize'
import { formatCount, plural } from '../format'
import { previewStyle } from '../lib/button-rows'
import { payload, sameKb, usedConditions, visibleRows } from '../lib/message-keyboard'
import type { KbCatalogs, KbCellOut, Settings, TextItem, TextsReply, TextVar } from '../types'
import { tplName } from '../lib/message-keyboard'
import { MessageKeyboardCard, type TplText } from './message-keyboard'

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
  // вкладка списка: сообщения или «части» (короткие тексты, которые подставляются в другие сообщения, например статусы)
  const [tabPick, setTabPick] = useState<'message' | 'plain'>()
  // свёрнутость групп списка: явный выбор админа; по умолчанию открыта только группа выбранного сообщения
  const [groupToggled, setGroupToggled] = useState<Record<string, boolean>>({})
  // черновики по ключу: переключение между текстами не теряет правки, но и не переносит их
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  // подписи кнопок шаблонов (button.tariff…) — стиль; текст лежит в drafts
  const [styleDrafts, setStyleDrafts] = useState<Record<string, string>>({})
  // тарифы и цена устройства для примера в превью; не загрузились — подставятся примеры переменных
  const [settings, setSettings] = useState<Settings>()
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
  useEffect(() => {
    get<Settings>('/api/settings').then(setSettings, () => undefined)
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
  const patch = (key: string, body: string, style?: string) =>
    setItems((list) => list?.map((i) => (i.key === key ? { ...i, body, overridden: body !== i.default, inactive: false, problem: undefined, style: style ?? i.style } : i)))
  const byKey = (key: string) => items.find((i) => i.key === key)
  const tplText = (key: string): TplText | undefined => {
    const i = byKey(key)
    return i && { label: draftOf(i), style: styleDrafts[key] ?? i.style, vars: i.vars.map((v) => varLabel(v.name)).join(', ') }
  }
  const tplDirty = (key: string) => {
    const i = byKey(key)
    return !!i && (draftOf(i) !== i.body || (styleDrafts[key] ?? i.style) !== i.style)
  }

  const own = cur?.vars ?? []
  const vars = [...globals, ...own]
  const samples = Object.fromEntries(vars.map((v) => [v.name, v.sample]))
  const raw = vars.filter((v) => v.raw).map((v) => v.name)

  const kb = cur?.kind === 'message' ? cur.keyboard : undefined
  const kbRows = kb ? (kbDrafts[cur.key] ?? kb.rows) : []
  const tplKeys = [...new Set(kbRows.flat().flatMap((c) => (c.template ? (cat.templates.find((t) => t.id === tplName(c))?.key ?? []) : [])))]
  const layoutDirty = !!kb && !sameKb(kbRows, kb.rows)
  const kbDirty = layoutDirty || tplKeys.some(tplDirty)
  const dropTplDraft = (k: string) => {
    dropDraft(k)
    setStyleDrafts((d) => {
      const rest = { ...d }
      delete rest[k]
      return rest
    })
  }
  const dropKbDraft = (key: string) => {
    setKbDrafts((d) => {
      const rest = { ...d }
      delete rest[key]
      return rest
    })
    tplKeys.forEach(dropTplDraft)
  }
  const condIds = kb ? usedConditions(kbRows, cat.actions) : []
  const fakeSet = new Set(fake)
  // Результат шаблона на примере: тарифы из настроек, число устройств с ценой за штуку.
  const expand = (c: KbCellOut) => {
    const t = cat.templates.find((x) => x.id === tplName(c))
    const txt = t && tplText(t.key)
    if (!t || !txt) return [[{ text: t?.title ?? c.id, style: previewStyle('') }]]
    const style = previewStyle(txt.style)
    const sample = (name: string) => byKey(t.key)?.vars.find((v) => v.name === name)?.sample ?? ''
    const items =
      t.id === 'devices'
        ? [1, 2, 3, 5].map((n) => ({ count: String(n), countShort: `${n} устр.`, price: String((settings?.devicePrice || Number(sample('price')) / 2) * n) }))
        : Object.entries(settings?.prices ?? { 1: Number(sample('price')) || 100 })
            .map(([m, p]) => ({ n: Number(m), p }))
            .sort((a, b) => a.n - b.n)
            .map(({ n, p }) => ({ months: `${n} ${plural(n, ['месяц', 'месяца', 'месяцев'])}`, monthsShort: `${n} мес.`, price: String(p) }))
    const buttons = items.map((v) => ({ text: renderPlain(txt.label, { ...samples, ...v }) || t.title, style }))
    return Array.from({ length: Math.ceil(buttons.length / 2) }, (_, i) => buttons.slice(i * 2, i * 2 + 2))
  }
  const previewRows = visibleRows(kbRows, fakeSet, cat.actions).flatMap((row) =>
    row[0]?.template
      ? expand(row[0])
      : [row.map((c) => ({ text: (c.label ? renderPlain(c.label, samples) : c.effective?.preview) || c.id, style: previewStyle(c.style || c.effective?.style || '') }))],
  )

  const tab = tabPick ?? (cur?.kind === 'plain' ? 'plain' : 'message')
  const s = q.trim().toLowerCase()
  const groups = new Map<string, TextItem[]>()
  for (const i of editable) {
    if (i.kind !== tab) continue
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
        // сначала раскладка: она проверяется сервером; подписи шаблонов общие для экранов и пишутся только после неё
        if (layoutDirty) await put(`${path(c)}/keyboard`, payload(kbRows))
        for (const key of tplKeys) {
          const b = byKey(key)
          if (!b || !tplDirty(key)) continue
          const body = draftOf(b)
          const style = styleDrafts[key] ?? b.style
          await put(path(b), { body, style })
          patch(key, body, style)
        }
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
      // клавиатура — ровно та, что в превью (с отмеченными условиями и раскрытыми шаблонами)
      await post(`${path(c)}/test`, previewRows.length ? { body, keyboard: previewRows.map((r) => r.map((b) => ({ text: b.text, style: b.style === 'default' ? '' : b.style }))) } : { body })
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
        <SegmentedControl
          block
          className="shrink-0"
          value={tab}
          options={[
            { value: 'message', label: 'Сообщения' },
            { value: 'plain', label: 'Части' },
          ]}
          onChange={(v) => {
            const next = v as 'message' | 'plain'
            setTabPick(next)
            const first = editable.find((i) => i.kind === next)
            if (first && cur?.kind !== next) setParams({ key: first.key })
          }}
        />
        {tab === 'plain' && <p className="footnote px-1 text-text-secondary">Короткие тексты, которые подставляются в другие сообщения (например, статус подписки).</p>}
        <SearchField placeholder={tab === 'plain' ? 'Найти часть' : 'Найти сообщение'} value={q} onChange={(e) => setQ(e.target.value)} onClear={() => setQ('')} />
        {/* Длинный список прокручивается сам: не отодвигает редактор и на компьютере закреплён рядом с ним. */}
        <div className="flex min-h-0 flex-col gap-4 overflow-y-auto max-lg:max-h-72">
          {!groups.size && <p className="admin-body px-1 text-text-secondary">Ничего не найдено</p>}
          {[...groups].map(([group, list]) => {
            // поиск раскрывает все группы с совпадениями; иначе — выбор админа или группа выбранного сообщения
            const open = s ? true : (groupToggled[group] ?? group === cur?.group)
            return (
              <div key={group} className="flex flex-col gap-1.5">
                <button
                  type="button"
                  aria-expanded={open}
                  className="admin-label focus-ring flex cursor-pointer items-center gap-1 rounded-sm px-1 text-left text-text-secondary"
                  onClick={() => setGroupToggled((t) => ({ ...t, [group]: !open }))}
                >
                  <Icon name={open ? 'chevron-down' : 'chevron-right'} size={14} strokeWidth={2.4} />
                  {group}
                </button>
                {open && (
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
                        onClick={() => {
                          // подписи шаблонов и условия превью относятся к открытой клавиатуре: на другом сообщении их не переносим
                          if (i.key !== cur?.key) {
                            setFake([])
                            tplKeys.forEach(dropTplDraft)
                          }
                          setParams({ key: i.key })
                        }}
                      >
                        <span className="flex min-w-0 grow flex-col">
                          <span className="font-medium">{i.title}</span>
                          <span className={cn('caption-1 truncate font-mono', !on && 'opacity-75')}>{i.key}</span>
                        </span>
                        {i.inactive && <StatusTag tone="destructive">не применяется</StatusTag>}
                        {(i.overridden || dirty) && (
                          <Tip label={dirty ? 'Есть несохранённые изменения' : 'Изменено'}>
                            <span role="img" aria-label={dirty ? 'Есть несохранённые изменения' : 'Изменено'} className={cn('size-2 flex-none rounded-full', on ? 'bg-on-accent-strong' : 'bg-warning')} />
                          </Tip>
                        )}
                      </button>
                    )
                  })}
                </div>
                )}
              </div>
            )
          })}
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
                tplText={tplText}
                onTplText={(key, ch) => {
                  if (ch.label !== undefined) setDrafts((d) => ({ ...d, [key]: ch.label as string }))
                  if (ch.style !== undefined) setStyleDrafts((d) => ({ ...d, [key]: ch.style as string }))
                }}
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
