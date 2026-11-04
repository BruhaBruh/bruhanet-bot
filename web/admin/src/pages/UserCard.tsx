import { Button, Cell, Info, Placeholder, Section, Spinner, Text, Title, VisuallyHidden } from '@telegram-apps/telegram-ui'
import { lazy, Suspense, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router'
import { ApiError, get } from '../api'
import { snippet } from '../editor/serialize'
import { formatDate, formatRub, panelStatusLabels } from '../format'
import type { DirectMessage, UserDetail } from '../types'
import { PaymentsTable } from './PaymentsTable'
import { UserAvatar } from './Users'

// tiptap — отдельным чанком
const Composer = lazy(() => import('../compose/Composer').then((m) => ({ default: m.Composer })))

const statusMark: Record<string, string> = { pending: '…', sent: '✓', failed: '✗' }

export default function UserCard() {
  const { id } = useParams()
  const [d, setD] = useState<UserDetail>()
  const [error, setError] = useState('')
  const [writing, setWriting] = useState(false)
  const [sent, setSent] = useState(false)

  useEffect(() => {
    get<UserDetail>(`/api/users/${id}`).then(setD, (e) => setError(e instanceof ApiError && e.status === 404 ? 'Юзер не найден' : 'Не удалось загрузить'))
  }, [id])

  function onSent(m: DirectMessage) {
    setD((d) => d && { ...d, messages: [m, ...d.messages] })
    setWriting(false)
    setSent(true)
  }

  const name = d ? (d.user.username ? '@' + d.user.username : String(d.user.telegramId)) : ''

  return (
    <>
      <Link to="/users" className="back">
        ← Юзеры
      </Link>
      {error && (
        <p className="tg-err pad" role="alert">
          {error}
        </p>
      )}
      {!d && !error && (
        <div className="tg-loading" role="status">
          <Spinner size="m" />
          <VisuallyHidden>Загрузка…</VisuallyHidden>
        </div>
      )}
      {d && (
        <>
          <div className="user-head">
            <UserAvatar name={name} size={48} />
            <div className="user-who">
              <Title level="2" weight="2" Component="h1">
                {name}
              </Title>
              <Text className="user-id">{d.user.telegramId}</Text>
            </div>
          </div>

          <Section header="Подписка">
            <Cell
              after={
                <Info type="text" subtitle={!d.panel.error && d.panel.status ? (panelStatusLabels[d.panel.status] ?? d.panel.status) : undefined}>
                  {d.panel.error ? 'панель недоступна' : d.panel.expireAt ? formatDate(d.panel.expireAt) : '—'}
                </Info>
              }
            >
              Подписка до
            </Cell>
          </Section>

          <Section header="Статистика">
            <Cell after={formatDate(d.user.createdAt)}>С нами с</Cell>
            <Cell after={`${d.stats.confirmed} на ${formatRub(d.stats.confirmedSum)}`}>Подтверждённых оплат</Cell>
            <Cell after={String(d.stats.unpaidAdvances)}>Неподтверждённых авансов</Cell>
            <Cell after={d.stats.firstConfirmedAt ? formatDate(d.stats.firstConfirmedAt) : '—'}>Первая оплата</Cell>
            <Cell after={d.stats.lastConfirmedAt ? formatDate(d.stats.lastConfirmedAt) : '—'}>Последняя оплата</Cell>
          </Section>

          <PaymentsTable header="Платежи" items={d.payments} />

          <Section header="Сообщения">
            <div className="pad">
              {writing ? (
                <Suspense fallback={<Spinner size="m" />}>
                  <Composer mode="direct" userId={d.user.telegramId} onSent={onSent} />
                </Suspense>
              ) : (
                <div className="write-row">
                  <Button
                    mode="bezeled"
                    onClick={() => {
                      setWriting(true)
                      setSent(false)
                    }}
                  >
                    Написать
                  </Button>
                  {sent && (
                    <span className="tg-ok" role="status">
                      Отправлено
                    </span>
                  )}
                </div>
              )}
            </div>
            {!d.messages.length ? (
              <Placeholder header="Сообщений нет">
                <span className="ph-emoji">✉️</span>
              </Placeholder>
            ) : (
              d.messages.map((m) => (
                <Cell key={m.id} multiline subtitle={formatDate(m.createdAt)} after={statusMark[m.status] ?? m.status}>
                  {snippet(m.text, m.photo)}
                </Cell>
              ))
            )}
          </Section>
        </>
      )}
    </>
  )
}
