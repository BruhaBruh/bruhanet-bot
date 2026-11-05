import { lazy, Suspense, useEffect, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router'
import { notWired } from '@/lib/not-wired'
import { Avatar, Button, Card, DataTable, Spinner, StatusTag, toast, type Tone } from '@/ui'
import { ApiError, get } from '../api'
import { snippet } from '../editor/serialize'
import { formatCount, formatFullDay, formatRub, formatShortDate, panelStatusLabels } from '../format'
import { usePageTitle } from '../layout/page-title'
import { userLabel } from '../promo'
import type { DirectMessage, UserDetail } from '../types'
import { payActions, userPayColumns, userPayMobileRow } from './payment-columns'

// tiptap — отдельным чанком
const Composer = lazy(() => import('../compose/Composer').then((m) => ({ default: m.Composer })))

const DASH = '—'
const PANEL_UNAVAILABLE = 'панель недоступна'
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'

const PANEL_TONES: Record<string, Tone> = {
  ACTIVE: 'success',
  DISABLED: 'destructive',
  LIMITED: 'warning',
  EXPIRED: 'neutral',
  not_found: 'neutral',
}

const MESSAGE_STATUS: Record<DirectMessage['status'], [Tone, string]> = {
  pending: ['warning', 'В очереди'],
  sent: ['success', 'Отправлено'],
  failed: ['destructive', 'Ошибка'],
}

const day = (iso: string | null) => (iso ? formatFullDay(iso) : DASH)
const panelStatusLabel = (status: string) => panelStatusLabels[status] ?? status

function PanelTag({ panel }: { panel: UserDetail['panel'] }) {
  if (panel.error) return <StatusTag tone="warning" dot>{PANEL_UNAVAILABLE}</StatusTag>
  if (!panel.status) return null
  return (
    <StatusTag tone={PANEL_TONES[panel.status] ?? 'neutral'} dot>
      {panelStatusLabel(panel.status)}
    </StatusTag>
  )
}

function Facts({ title, rows }: { title: string; rows: Array<[string, ReactNode]> }) {
  return (
    <section className="rounded-lg bg-surface px-5 pt-4 pb-2 shadow-card">
      <h3 className="admin-label mb-1 text-text-secondary">{title}</h3>
      <dl>
        {rows.map(([k, v]) => (
          <div key={k} className={`flex justify-between gap-3 py-2.5 ${separator}`}>
            <dt className="text-text-secondary">{k}</dt>
            <dd className="text-right font-medium">{v}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

// key по id: при переходе на другого юзера стейт (данные, композер) начинается заново
export default function UserCard() {
  const { id } = useParams()
  return <UserCardInner key={id} id={id} />
}

function UserCardInner({ id }: { id?: string }) {
  const navigate = useNavigate()
  const [d, setD] = useState<UserDetail>()
  const [error, setError] = useState('')
  const [writing, setWriting] = useState(false)

  useEffect(() => {
    get<UserDetail>(`/api/users/${id}`).then(setD, (e) =>
      setError(e instanceof ApiError && e.status === 404 ? 'Пользователь не найден' : 'Не удалось загрузить пользователя'),
    )
  }, [id])

  const name = d && userLabel(d.user)
  usePageTitle(name)

  function onSent(m: DirectMessage) {
    setD((d) => d && { ...d, messages: [m, ...d.messages] })
    setWriting(false)
    toast('Сообщение отправлено', { icon: 'check' })
  }

  const back = (
    <div className="-my-2 -ml-2">
      <Button variant="plain" icon="chevron-left" onClick={() => navigate('/users')}>
        Все пользователи
      </Button>
    </div>
  )

  if (!d) {
    return (
      <>
        {back}
        {error ? (
          <p role="alert" className="admin-body text-destructive-strong">
            {error}
          </p>
        ) : (
          <Spinner className="self-center p-8" />
        )}
      </>
    )
  }

  const { user, stats, panel, messages } = d
  // Средний чек — по подтверждённым: только у них есть сумма в stats.
  const average = stats.confirmed ? formatRub(Math.round(stats.confirmedSum / stats.confirmed)) : DASH

  return (
    <>
      {back}
      <section className="admin-body flex flex-wrap items-center gap-5 rounded-lg bg-surface p-6 shadow-card">
        <Avatar name={name} id={user.telegramId} size={64} />
        <div className="flex min-w-0 flex-[1_1_--spacing(70)] flex-col gap-1.5">
          <div className="flex flex-wrap items-center gap-2.5">
            <h2 className="title-2 min-w-0 break-all">{name}</h2>
            <PanelTag panel={panel} />
          </div>
          <div className="admin-table flex flex-wrap gap-x-4 gap-y-1.5 text-text-secondary">
            {user.username && (
              <a className="focus-ring break-all text-accent-strong" href={`https://t.me/${user.username}`} target="_blank" rel="noreferrer">
                @{user.username}
              </a>
            )}
            <span>
              ID <code className="font-mono text-[13px] text-text">{user.telegramId}</code>
            </span>
            <span>с нами с {formatFullDay(user.createdAt)}</span>
            <span>язык: {DASH}</span>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="tinted" aria-expanded={writing} onClick={() => setWriting(!writing)}>
            Написать
          </Button>
          <Button variant="gray" icon="gift" onClick={notWired}>
            Выдать дни
          </Button>
          <Button variant="destructive" icon="block" onClick={notWired}>
            Заблокировать
          </Button>
        </div>
      </section>

      {writing && (
        <Card title="Новое сообщение">
          <Suspense fallback={<Spinner className="self-center p-8" />}>
            <Composer mode="direct" userId={user.telegramId} onSent={onSent} />
          </Suspense>
        </Card>
      )}

      <div className="admin-body grid grid-cols-[repeat(auto-fit,minmax(min(100%,--spacing(70)),1fr))] gap-4">
        <Facts
          title="Подписка"
          rows={[
            ['Срок до', panel.error ? PANEL_UNAVAILABLE : day(panel.expireAt)],
            ['Статус', !panel.error && panel.status ? panelStatusLabel(panel.status) : DASH],
            ['Тариф', DASH],
            ['Устройства', DASH],
            ['Автопродление', DASH],
          ]}
        />
        <Facts
          title="Финансы"
          rows={[
            ['Оплачено всего', formatRub(stats.confirmedSum)],
            ['Платежей', formatCount(stats.confirmed)],
            ['Средний чек', average],
            ['Промокоды', DASH],
            ['Неоплаченные авансы', formatCount(stats.unpaidAdvances)],
            ['Первая оплата', day(stats.firstConfirmedAt)],
            ['Последняя оплата', day(stats.lastConfirmedAt)],
          ]}
        />
        <Facts
          title="Активность"
          rows={[
            ['Последний визит', DASH],
            ['Откуда пришёл', DASH],
            ['Пригласил друзей', DASH],
            ['Получил рассылок', DASH],
          ]}
        />
      </div>

      <div className="overflow-x-auto">
        <DataTable
          title="Платежи"
          columns={userPayColumns}
          rows={d.payments}
          rowActions={payActions}
          mobileRow={userPayMobileRow}
          empty="У пользователя пока нет платежей"
        />
      </div>

      <Card title="Сообщения" className="admin-body">
        {!messages.length ? (
          <p className="text-text-secondary">Сообщений пока нет</p>
        ) : (
          <ul>
            {messages.map((m) => {
              const [tone, label] = MESSAGE_STATUS[m.status] ?? ['neutral', m.status]
              return (
                <li key={m.id} className={`flex items-center justify-between gap-3 py-2.5 ${separator}`}>
                  <span className="flex min-w-0 flex-col">
                    <span className="break-words">{snippet(m.text, m.photo)}</span>
                    <span className="footnote text-text-secondary">{formatShortDate(m.createdAt)}</span>
                  </span>
                  <StatusTag tone={tone}>{label}</StatusTag>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
    </>
  )
}
