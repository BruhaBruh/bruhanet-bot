import { lazy, Suspense, useEffect, useState, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { TgAvatar, Button, Card, DataTable, RowAction, Spinner, StatusTag, toast, type Tone } from '@/ui'
import { ApiError, get } from '../api'
import { snippet } from '../editor/serialize'
import { formatCount, formatFullDay, formatRub, formatShortDate, panelStatusLabels, panelStatusTones } from '../format'
import { usePageTitle, useSyncKey } from '../layout/page-title'
import { userLabel, userTagUrl } from '../promo'
import { UserChip } from './user-chip'
import type { DirectMessage, UserDetail } from '../types'
import { copyText } from '../lib/clipboard'
import { DevicesCard } from './user-devices'
import { userPayColumns, userPayMobileRow } from './payment-columns'
import { usePaymentReview } from './payment-review'
import { BlockDialog, GrantDialog } from './user-actions'

// tiptap — отдельным чанком
const Composer = lazy(() => import('../compose/Composer').then((m) => ({ default: m.Composer })))

const DASH = '—'
const PANEL_UNAVAILABLE = 'панель недоступна'
const separator = 'shadow-[inset_0_-0.5px_0_var(--separator)]'

const MESSAGE_STATUS: Record<DirectMessage['status'], [Tone, string]> = {
  pending: ['warning', 'В очереди'],
  sent: ['success', 'Отправлено'],
  failed: ['destructive', 'Ошибка'],
}

const day = (iso: string | null) => (iso ? formatFullDay(iso) : DASH)
const panelStatusLabel = (status: string) => panelStatusLabels[status] ?? status

async function copyId(id: number) {
  try {
    await navigator.clipboard.writeText(String(id))
    toast('ID скопирован', { icon: 'check' })
  } catch {
    toast('Не удалось скопировать ID')
  }
}

async function copyLink(url: string) {
  if (await copyText(url)) toast('Ссылка скопирована', { icon: 'check' })
  else toast('Не удалось скопировать ссылку')
}

function PanelTag({ panel }: { panel: UserDetail['panel'] }) {
  if (panel.error) return <StatusTag tone="warning" dot>{PANEL_UNAVAILABLE}</StatusTag>
  if (!panel.status) return null
  return (
    <StatusTag tone={panelStatusTones[panel.status] ?? 'neutral'} dot>
      {panelStatusLabel(panel.status)}
    </StatusTag>
  )
}

/** «5 + 2 = 7» из нашей базы; если лимит в самой панели другой (ручная правка), рядом «в панели: N» с подсветкой. */
function DeviceFact({ base, extra, panel }: { base: number; extra: number; panel?: number }) {
  const total = base + extra
  return (
    <span className="inline-flex flex-wrap items-baseline justify-end gap-x-2">
      <span>{`${base} + ${extra} = ${total}`}</span>
      {panel !== undefined && panel !== total && (
        <span className="rounded-xs bg-warning-tint px-1.5 text-warning-strong" title="Лимит в Remnawave отличается от нашего: правили вручную?">
          в панели: {panel}
        </span>
      )}
    </span>
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
            <dd className="min-w-0 text-right font-medium">{v}</dd>
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
  // hidden — скрыт, но смонтирован: черновик не пропадает при повторном клике.
  const [composer, setComposer] = useState<'none' | 'open' | 'hidden'>('none')
  const [dialog, setDialog] = useState<'grant' | 'block'>()

  const syncKey = useSyncKey()
  const [reloads, setReloads] = useState(0)
  const reload = () => setReloads((n) => n + 1)
  const review = usePaymentReview(reload)
  useEffect(() => {
    get<UserDetail>(`/api/users/${id}`).then(setD, (e) =>
      setError(e instanceof ApiError && e.status === 404 ? 'Пользователь не найден' : 'Не удалось загрузить пользователя'),
    )
  }, [id, syncKey, reloads])

  const name = d && userLabel(d.user)
  usePageTitle(name)

  function onSent(m: DirectMessage) {
    setD((d) => d && { ...d, messages: [m, ...d.messages] })
    setComposer('hidden')
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

  const { user, stats, panel, messages, promos, grants, devices, referral } = d
  // Средний чек — по подтверждённым: только у них есть сумма в stats.
  const average = stats.confirmed ? formatRub(Math.round(stats.confirmedSum / stats.confirmed)) : DASH

  return (
    <>
      {back}
      <section className="admin-body flex flex-wrap items-center gap-5 rounded-lg bg-surface p-6 shadow-card">
        <TgAvatar tg={user.telegramId} name={name} id={user.telegramId} size={64} />
        <div className="flex min-w-0 flex-[1_1_--spacing(70)] flex-col gap-1.5">
          <div className="flex flex-wrap items-center gap-2.5">
            <h2 className="title-2 min-w-0 break-all">{name}</h2>
            <PanelTag panel={panel} />
            {user.blockedAt && (
              <StatusTag tone="destructive" dot>
                Заблокирован
              </StatusTag>
            )}
            {user.blockedBotAt && (
              <StatusTag tone="destructive" dot>
                заблокировал бота
              </StatusTag>
            )}
          </div>
          <div className="admin-table flex flex-wrap gap-x-4 gap-y-1.5 text-text-secondary">
            {user.username && (
              <a className="focus-ring break-all text-accent-strong" href={userTagUrl(user) ?? undefined} target="_blank" rel="noopener noreferrer">
                @{user.username}
              </a>
            )}
            <button
              type="button"
              className="focus-ring cursor-pointer"
              aria-label="Скопировать ID"
              onClick={() => copyId(user.telegramId)}
            >
              ID <code className="mono text-text">{user.telegramId}</code>
            </button>
            <span>с нами с {formatFullDay(user.createdAt)}</span>
          </div>
          {panel.subscriptionUrl && (
            <div>
              <Button size="sm" variant="gray" icon="copy" onClick={() => void copyLink(panel.subscriptionUrl!)}>
                Скопировать ссылку подписки
              </Button>
            </div>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="tinted" aria-expanded={composer === 'open'} onClick={() => setComposer(composer === 'open' ? 'hidden' : 'open')}>
            Написать
          </Button>
          <Button variant="gray" icon="gift" onClick={() => setDialog('grant')}>
            Выдать дни
          </Button>
          <Button variant="destructive" icon="block" onClick={() => setDialog('block')}>
            {user.blockedAt ? 'Снять блокировку' : 'Заблокировать'}
          </Button>
        </div>
      </section>

      {composer !== 'none' && (
        <div hidden={composer === 'hidden'}>
          <Card title="Новое сообщение">
            <Suspense fallback={<Spinner className="self-center p-8" />}>
              <Composer mode="direct" userId={user.telegramId} onSent={onSent} />
            </Suspense>
          </Card>
        </div>
      )}

      <div className="admin-body grid grid-cols-[repeat(auto-fit,minmax(min(100%,--spacing(70)),1fr))] gap-4">
        <Facts
          title="Подписка"
          rows={[
            [
              'ID в панели', // кнопка 32px выше строки в 24px: -my-1.5 держит высоту строки как у соседних
              panel.id == null ? (
                DASH
              ) : (
                <span className="-my-1.5 flex items-center justify-end gap-1">
                  <code className="mono break-all">{panel.id}</code>
                  <RowAction icon="copy" label="Скопировать ID в панели" onClick={() => void copyId(panel.id!)} />
                </span>
              ),
            ],
            ['Статус', !panel.error && panel.status ? panelStatusLabel(panel.status) : DASH],
            ['Срок до', panel.error ? PANEL_UNAVAILABLE : day(panel.expireAt)],
            ['Тариф', user.tariffMonths ? `${user.tariffMonths} мес.` : DASH],
            ['Пробный период', user.trialUsedAt ? `использован ${day(user.trialUsedAt)}` : DASH],
            ['Устройства', devices ? <DeviceFact base={devices.base} extra={devices.extra} panel={panel.deviceLimit} /> : DASH],
          ]}
        />
        <Facts
          title="Финансы"
          rows={[
            ['Платежей', formatCount(stats.confirmed)],
            ['Оплачено всего', formatRub(stats.confirmedSum)],
            ['Средний чек', average],
            ['Первая оплата', day(stats.firstConfirmedAt)],
            ['Последняя оплата', day(stats.lastConfirmedAt)],
            ['Промокоды', promos.length ? <span className="mono break-all">{promos.map((p) => p.code).join(', ')}</span> : DASH],
            ['Неоплаченные авансы', formatCount(stats.unpaidAdvances)],
          ]}
        />
        <Facts
          title="Активность"
          rows={[
            ['Последний визит', user.lastSeenAt ? formatShortDate(user.lastSeenAt) : DASH],
            ['Получил рассылок', formatCount(user.broadcastsReceived)],
            ...(referral.referredBy == null
              ? []
              : [
                  [
                    'Кем приглашён',
                    <Link key="by" className="focus-ring break-all text-accent-strong" to={`/users/${referral.referredBy}`}>
                      <UserChip id={referral.referredBy} label={userLabel({ name: referral.referredByName, username: referral.referredByUsername, telegramId: referral.referredBy })} />
                    </Link>,
                  ] as [string, ReactNode],
                ]),
            ['Пригласил друзей', `${formatCount(referral.invited)} (оплатили ${formatCount(referral.paid)})`],
          ]}
        />
      </div>

      <DevicesCard userId={user.telegramId} reloadKey={syncKey + reloads} />

      <div className="overflow-x-auto">
        <DataTable
          title="Платежи"
          columns={userPayColumns}
          rows={d.payments}
          rowActions={review.rowActions}
          expandedRow={review.expandedRow}
          mobileRow={(p) => ({ ...userPayMobileRow(p), chevron: true, onClick: () => review.toggle(p) })}
          empty="У пользователя пока нет платежей"
        />
      </div>

      {grants.length > 0 && (
        <Card title="Выдачи" className="admin-body">
          <ul>
            {grants.map((g) => (
              <li key={g.id} className={`flex items-center justify-between gap-3 py-2.5 ${separator}`}>
                <span className="text-text-secondary">{formatShortDate(g.createdAt)}</span>
                <span className="font-medium">+{g.days} дн.</span>
              </li>
            ))}
          </ul>
        </Card>
      )}

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
                    <span className="break-words">{snippet(m.text, m.photo || m.photoId || undefined)}</span>
                    <span className="footnote text-text-secondary">{formatShortDate(m.createdAt)}</span>
                  </span>
                  <StatusTag tone={tone}>{label}</StatusTag>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
      {review.dialogs}
      {dialog === 'grant' && (
        <GrantDialog user={user} expireAt={panel.expireAt ?? user.expireAt} onClose={() => setDialog(undefined)} onDone={reload} />
      )}
      {dialog === 'block' && <BlockDialog user={user} onClose={() => setDialog(undefined)} onDone={reload} />}
    </>
  )
}
