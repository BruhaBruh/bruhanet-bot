import { useState } from 'react'
import { Button, Sheet, toast } from '@/ui'
import { ApiError, del, put } from '../api'
import type { TextItem } from '../types'
import { ButtonFields } from './keyboard-editor'

const path = (key: string) => `/api/texts/${encodeURIComponent(key)}`

/** Окно «Системные кнопки»: кнопки, которых нет ни в одной раскладке. Подпись и стиль — по одной, один запрос за раз. */
export function SystemButtons({ items, onClose, onChanged }: { items: TextItem[]; onClose(): void; onChanged(): Promise<unknown> }) {
  const [labels, setLabels] = useState<Record<string, string>>({})
  const [styles, setStyles] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState<string>()
  const [error, setError] = useState<{ key: string; message: string }>()

  const label = (i: TextItem) => labels[i.key] ?? i.body
  const style = (i: TextItem) => styles[i.key] ?? i.style
  const dirty = (i: TextItem) => label(i) !== i.body || style(i) !== i.style
  const drop = (key: string) => {
    for (const set of [setLabels, setStyles])
      set((d) => {
        const rest = { ...d }
        delete rest[key]
        return rest
      })
  }

  async function run(i: TextItem, fn: () => Promise<void>, done: string) {
    if (busy) return
    setBusy(i.key)
    setError(undefined)
    try {
      await fn()
      drop(i.key)
      await onChanged()
      toast(done, { icon: 'check' })
    } catch (e) {
      setError({ key: i.key, message: e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось выполнить, попробуйте ещё раз' })
    } finally {
      setBusy(undefined)
    }
  }

  return (
    <Sheet title="Системные кнопки" onClose={onClose}>
      <div className="flex flex-col gap-3 px-4">
        <p className="footnote text-text-secondary">
          Кнопки, которых нет в раскладках клавиатур. Действие задаёт код бота; подпись и стиль общие для всех экранов с этой кнопкой.
        </p>
        {items.map((i) => (
          <div key={i.key} className="flex min-w-0 flex-col gap-2 rounded-md bg-fill-tertiary p-3">
            <ButtonFields
              id={i.key}
              view={{ title: i.title, label: label(i), style: style(i), route: i.route }}
              busy={!!busy}
              onLabel={(v) => setLabels((d) => ({ ...d, [i.key]: v }))}
              onStyle={(v) => setStyles((d) => ({ ...d, [i.key]: v }))}
            />
            {error?.key === i.key && (
              <p role="alert" className="footnote text-destructive-strong">
                {error.message}
              </p>
            )}
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                variant="gray"
                size="sm"
                icon="refresh"
                disabled={!!busy || !i.overridden}
                aria-label={`Вернуть как в коде: ${i.title}`}
                onClick={() => run(i, () => del(path(i.key)), 'Кнопка возвращена как в коде')}
              >
                Вернуть как в коде
              </Button>
              <Button
                size="sm"
                loading={busy === i.key}
                disabled={!!busy || !dirty(i)}
                aria-label={`Сохранить кнопку ${i.title}`}
                onClick={() => run(i, () => put(path(i.key), { body: label(i), style: style(i) }), `Кнопка «${i.title}» сохранена`)}
              >
                Сохранить
              </Button>
            </div>
          </div>
        ))}
      </div>
    </Sheet>
  )
}
