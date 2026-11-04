import { Cell, Placeholder, Section, Spinner, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useCallback, useEffect, useRef, useState } from 'react'
import { get } from '../api'
import { Composer } from '../compose/Composer'
import { snippet } from '../editor/serialize'
import { formatDate } from '../format'
import type { Broadcast } from '../types'

const stats = (b: Broadcast) => `доставлено ${b.sent} из ${b.total}, не доставлено ${b.failed}`

export default function Broadcasts() {
  const [items, setItems] = useState<Broadcast[]>()
  const [loadError, setLoadError] = useState(false)
  const latest = useRef(items)
  latest.current = items

  // ponytail: только первая страница (20 последних), пагинация — когда история понадобится глубже
  const load = useCallback(async () => {
    try {
      setItems((await get<{ items: Broadcast[] }>('/api/broadcasts?page=1')).items)
      setLoadError(false)
    } catch {
      setLoadError(true)
    }
  }, [])

  useEffect(() => {
    void load()
    // автообновление, пока есть незавершённые и вкладка видна
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible' && latest.current?.some((b) => !b.finishedAt)) void load()
    }, 5000)
    return () => clearInterval(timer)
  }, [load])

  return (
    <>
      <VisuallyHidden Component="h1">Рассылки</VisuallyHidden>
      <Section header="Новая рассылка">
        <div className="pad">
          <Composer mode="broadcast" onSent={load} />
        </div>
      </Section>

      <Section header="История">
        {loadError ? (
          <p className="tg-err pad" role="alert">
            Не удалось загрузить
          </p>
        ) : !items ? (
          <div className="tg-loading" role="status">
            <Spinner size="m" />
            <VisuallyHidden>Загрузка…</VisuallyHidden>
          </div>
        ) : !items.length ? (
          <Placeholder header="Рассылок ещё не было">
            <span className="ph-emoji">📣</span>
          </Placeholder>
        ) : (
          items.map((b) => (
            <Cell key={b.id} multiline subtitle={`${formatDate(b.createdAt)} · ${b.finishedAt ? stats(b) : 'идёт… ' + stats(b)}`}>
              {snippet(b.text, b.photo)}
            </Cell>
          ))
        )}
      </Section>
    </>
  )
}
