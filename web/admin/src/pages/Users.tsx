import { Avatar, Cell, Info, Input, Navigation, Placeholder, Section, Spinner, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { useEffect, useRef, useState } from 'react'
import { get } from '../api'
import { formatMonthYear, formatRub, plural } from '../format'
import { RowLink } from '../layout/RowLink'
import type { UserSummary } from '../types'

// Буква и цвет аватарки детерминированно по имени (как Telegram — по id).
export function UserAvatar({ name, size = 40 }: { name: string; size?: 40 | 48 }) {
  let h = 0
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) >>> 0
  return <Avatar size={size} acronym={name.replace(/^@/, '').slice(0, 1).toUpperCase()} className={`av av-letter av-${(h % 6) + 1}`} aria-hidden="true" />
}

export default function Users() {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<UserSummary[]>()
  const [error, setError] = useState(false)
  const seq = useRef(0)
  const first = useRef(true)

  useEffect(() => {
    const load = async () => {
      const n = ++seq.current
      try {
        const r = await get<{ items: UserSummary[] }>(`/api/users?q=${encodeURIComponent(q.trim())}`)
        if (n === seq.current) {
          setItems(r.items)
          setError(false)
        }
      } catch {
        if (n === seq.current) setError(true)
      }
    }
    // первая загрузка сразу, поиск — с debounce 300 мс
    if (first.current) {
      first.current = false
      void load()
      return
    }
    const t = setTimeout(load, 300)
    return () => clearTimeout(t)
  }, [q])

  return (
    <>
      <VisuallyHidden Component="h1">Юзеры</VisuallyHidden>
      <Section>
        <Input before="🔍" placeholder="ID или @username" aria-label="Поиск" value={q} onChange={(e) => setQ(e.target.value)} />
      </Section>
      {error && (
        <p className="tg-err pad" role="alert">
          Не удалось загрузить
        </p>
      )}
      {!items && !error ? (
        <div className="tg-loading" role="status">
          <Spinner size="m" />
          <VisuallyHidden>Загрузка…</VisuallyHidden>
        </div>
      ) : items && !items.length && !error ? (
        <Placeholder header="Никого нет">
          <span className="ph-emoji">🔍</span>
        </Placeholder>
      ) : (
        items && (
          <Section header={`Найдено: ${items.length}`}>
            {items.map((u) => {
              const name = u.username ? '@' + u.username : String(u.telegramId)
              return (
                <RowLink key={u.telegramId} to={`/users/${u.telegramId}`}>
                  <Cell
                    className="keep-after"
                    before={<UserAvatar name={name} />}
                    subtitle={`${u.telegramId} · с ${formatMonthYear(u.createdAt)}`}
                    after={
                      <Navigation>
                        <Info type="text" subtitle={`${u.confirmed} ${plural(u.confirmed, ['оплата', 'оплаты', 'оплат'])}`}>
                          {formatRub(u.confirmedSum)}
                        </Info>
                      </Navigation>
                    }
                  >
                    {name}
                  </Cell>
                </RowLink>
              )
            })}
          </Section>
        )
      )}
    </>
  )
}
