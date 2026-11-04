import { Button, Cell, Input, Placeholder, Section, Spinner, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router'
import { ApiError, del, get, post, put } from '../api'
import { TgEditor, type TgEditorHandle } from '../editor/TgEditor'
import { renderPlain, renderPreview, sanitize } from '../editor/serialize'
import { RowLink } from '../layout/RowLink'
import type { TextItem } from '../types'

const varLabel = (name: string) => `{{.${name}}}`
const path = (c: TextItem) => `/api/texts/${encodeURIComponent(c.key)}`

export default function Texts() {
  const [params] = useSearchParams()
  const [items, setItems] = useState<TextItem[]>()
  const [loadError, setLoadError] = useState(false)
  const [q, setQ] = useState('')
  // черновики по ключу: переключение между текстами не теряет правки, но и не переносит их
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [error, setError] = useState('')
  const [sent, setSent] = useState(false)
  const [busy, setBusy] = useState(false)
  const inFlight = useRef(false)
  const editor = useRef<TgEditorHandle>(null)
  const plain = useRef<HTMLInputElement>(null)

  useEffect(() => {
    get<{ items: TextItem[] }>('/api/texts').then(
      (r) => setItems(r.items),
      () => setLoadError(true),
    )
  }, [])

  const cur = items?.find((i) => i.key === params.get('key')) ?? items?.[0]
  useEffect(() => {
    setError('')
    setSent(false)
  }, [cur?.key])
  useEffect(() => {
    if (!sent) return
    const t = setTimeout(() => setSent(false), 3000)
    return () => clearTimeout(t)
  }, [sent])

  if (loadError)
    return (
      <p className="tg-err pad" role="alert">
        Не удалось загрузить
      </p>
    )
  if (!items)
    return (
      <div className="tg-loading" role="status">
        <Spinner size="m" />
        <VisuallyHidden>Загрузка…</VisuallyHidden>
      </div>
    )

  const isDirty = (i: TextItem) => drafts[i.key] !== undefined && drafts[i.key] !== i.body
  const draft = cur ? (drafts[cur.key] ?? cur.body) : ''
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
    if (s && !i.title.toLowerCase().includes(s) && !i.body.toLowerCase().includes(s)) continue
    groups.set(i.group, [...(groups.get(i.group) ?? []), i])
  }

  const samples = Object.fromEntries((cur?.vars ?? []).map((v) => [v.name, v.sample]))
  const raw = (cur?.vars ?? []).filter((v) => v.raw).map((v) => v.name)

  function insertVar(name: string) {
    const text = varLabel(name)
    if (cur?.kind === 'message') return editor.current?.insert(text)
    const at = plain.current?.selectionStart ?? draft.length
    const end = plain.current?.selectionEnd ?? at
    setDraft(draft.slice(0, at) + text + draft.slice(end))
  }

  // один запрос за раз: двойной клик не шлёт второй
  async function run(fn: () => Promise<void>) {
    if (inFlight.current) return
    inFlight.current = true
    setError('')
    setSent(false)
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      setError(e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось выполнить, попробуйте ещё раз')
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }

  const save = (c: TextItem, body: string) =>
    run(async () => {
      await put(path(c), { body })
      patch(c.key, body)
      dropDraft(c.key)
    })
  const sendTest = (c: TextItem, body: string) =>
    run(async () => {
      await post(`${path(c)}/test`, { body })
      setSent(true)
    })
  const reset = (c: TextItem) =>
    run(async () => {
      await del(path(c))
      patch(c.key, c.default)
      dropDraft(c.key)
    })

  return (
    <>
      <VisuallyHidden Component="h1">Тексты</VisuallyHidden>
      <div className="texts">
        <div className="texts-list">
          <Section>
            <Input before="🔍" placeholder="Поиск" aria-label="Поиск" value={q} onChange={(e) => setQ(e.target.value)} />
          </Section>
          {!groups.size && (
            <Placeholder header="Ничего не найдено">
              <span className="ph-emoji">🔍</span>
            </Placeholder>
          )}
          {[...groups].map(([group, list]) => (
            <Section key={group} header={group}>
              {list.map((i) => (
                <RowLink key={i.key} to={`?key=${encodeURIComponent(i.key)}`} current={i.key === cur?.key}>
                  <Cell multiline subtitle={isDirty(i) ? 'есть несохранённые изменения' : i.overridden ? 'изменён' : undefined}>
                    {i.title}
                  </Cell>
                </RowLink>
              ))}
            </Section>
          ))}
        </div>

        {cur && (
          <div className="texts-edit">
            <Section
              header={cur.title}
              footer={
                [cur.kind !== 'message' && 'Простой текст без разметки, до 64 символов.', cur.overridden && 'изменён', isDirty(cur) && 'есть несохранённые изменения']
                  .filter(Boolean)
                  .join(' · ') || undefined
              }
            >
              {cur.kind === 'message' ? (
                <div className="pad">
                  <TgEditor key={cur.key} ref={editor} value={draft} onChange={setDraft} disabled={busy} />
                </div>
              ) : (
                <Input ref={plain} aria-label={cur.title} placeholder={cur.title} maxLength={64} value={draft} onChange={(e) => setDraft(e.target.value)} />
              )}
            </Section>

            {cur.vars.length > 0 && (
              <Section header="Переменные (клик — вставить)">
                <div className="pad texts-row">
                  {cur.vars.map((v) => (
                    <Button key={v.name} mode="bezeled" size="s" title={v.description} onClick={() => insertVar(v.name)}>
                      {varLabel(v.name)}
                    </Button>
                  ))}
                </div>
              </Section>
            )}

            <Section header="Превью">
              <div className="pad">
                {cur.kind === 'message' ? (
                  // только разрешённые теги после sanitize
                  <div className="tg-msg" dangerouslySetInnerHTML={{ __html: sanitize(renderPreview(draft, samples, raw)) }} />
                ) : (
                  <div className="tg-msg">{renderPlain(draft, samples)}</div>
                )}
              </div>
            </Section>

            {error && (
              <p className="tg-err" role="alert">
                {error}
              </p>
            )}
            {sent && (
              <p className="tg-ok" role="status">
                Отправлено в Telegram
              </p>
            )}

            <div className="texts-row texts-actions">
              <Button loading={busy} disabled={!isDirty(cur)} onClick={() => save(cur, draft)}>
                Сохранить
              </Button>
              <Button mode="bezeled" disabled={busy} onClick={() => sendTest(cur, draft)}>
                Отправить себе
              </Button>
              {isDirty(cur) && (
                <Button
                  mode="plain"
                  disabled={busy}
                  onClick={() => {
                    dropDraft(cur.key)
                    setError('')
                  }}
                >
                  Отменить
                </Button>
              )}
              {cur.overridden && (
                <Button mode="plain" disabled={busy} onClick={() => reset(cur)}>
                  Сбросить к стандартному
                </Button>
              )}
            </div>
          </div>
        )}
      </div>
    </>
  )
}
