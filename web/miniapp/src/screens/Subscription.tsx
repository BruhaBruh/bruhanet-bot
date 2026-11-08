import { useCallback, useEffect, useState } from 'react'
import { StatusTag, type Tone } from '@/ui/status-tag'
import { ApiError, get } from '~/lib/api'
import { days, formatBytes, formatDate } from '~/lib/format'
import { Connect } from './Connect'
import { Hosts } from './Hosts'
import { PayBanner } from './PayBanner'
import { ErrorBlock, Loading } from './states'

type State = 'none' | 'active' | 'expired' | 'disabled'
interface Sub {
  state: State
  expireAt: string | null
  daysLeft: number
  trafficUsedBytes: number
  trafficLimitBytes: number | null
  devices: { used: number | null; limit: number | null }
}

const TAGS: Record<State, { tone: Tone; label: string }> = {
  active: { tone: 'success', label: 'Активна' },
  expired: { tone: 'destructive', label: 'Закончилась' },
  disabled: { tone: 'warning', label: 'Отключена' },
  none: { tone: 'neutral', label: 'Нет подписки' },
}

type Load = { status: 'loading' } | { status: 'ready'; sub: Sub } | { status: 'panelDown' } | { status: 'error' }

export default function Subscription() {
  const [load, setLoad] = useState<Load>({ status: 'loading' })

  const fetchSub = useCallback(() => {
    get<Sub>('/m/api/subscription').then(
      (sub) => setLoad({ status: 'ready', sub }),
      (e: unknown) => setLoad({ status: e instanceof ApiError && e.status === 502 ? 'panelDown' : 'error' }),
    )
  }, [])
  useEffect(fetchSub, [fetchSub])

  const retry = () => {
    setLoad({ status: 'loading' })
    fetchSub()
  }

  if (load.status === 'loading') return <Loading />
  if (load.status === 'panelDown') return <ErrorBlock title="Панель не отвечает" text="Попробуй чуть позже." onRetry={retry} />
  if (load.status === 'error') return <ErrorBlock title="Не получилось загрузить" onRetry={retry} />

  const { sub } = load
  const tag = TAGS[sub.state]
  const limit = sub.trafficLimitBytes
  return (
    <div className="pt-4">
      <section className="mx-4 mb-4 flex flex-col gap-3 rounded-lg bg-surface p-4">
        <div className="flex items-center justify-between">
          <h2 className="headline">Подписка</h2>
          <StatusTag tone={tag.tone}>{tag.label}</StatusTag>
        </div>
        {sub.state === 'none' ? (
          <p className="subheadline text-text-secondary">Подписки пока нет. Оформить её можно в боте.</p>
        ) : (
          <>
            {sub.state === 'active' && <div className="title-2 font-semibold">{days(sub.daysLeft)}</div>}
            {sub.expireAt && (
              <p className="subheadline text-text-secondary">
                {sub.state === 'active' ? 'до' : 'закончилась'} {formatDate(sub.expireAt)}
              </p>
            )}
            <div className="flex flex-col gap-1.5">
              <div className="flex justify-between">
                <span className="subheadline text-text-secondary">Трафик</span>
                <span className="subheadline">
                  {limit ? `${formatBytes(sub.trafficUsedBytes)} из ${formatBytes(limit)}` : `${formatBytes(sub.trafficUsedBytes)}, без лимита`}
                </span>
              </div>
              {limit && (
                <div className="h-1.5 overflow-hidden rounded-full bg-fill-tertiary" role="presentation">
                  <div className="h-full rounded-full bg-accent" style={{ width: `${Math.min(100, (sub.trafficUsedBytes / limit) * 100)}%` }} />
                </div>
              )}
            </div>
            <div className="flex justify-between">
              <span className="subheadline text-text-secondary">Устройства</span>
              <span className="subheadline">
                {sub.devices.used ?? '—'} из {sub.devices.limit ?? '—'}
              </span>
            </div>
          </>
        )}
      </section>
      <PayBanner />
      {sub.state === 'active' && <Connect />}
      {sub.state === 'active' && <Hosts />}
    </div>
  )
}
