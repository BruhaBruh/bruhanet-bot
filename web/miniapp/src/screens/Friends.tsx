import { useCallback, useEffect, useState } from 'react'
import { copyText } from '@/lib/clipboard'
import { Button } from '@/ui/primitives/button'
import { get } from '~/lib/api'
import { days } from '~/lib/format'
import { shareLink } from '~/lib/telegram'
import { ErrorBlock, Loading } from './states'

interface Referral {
  enabled: boolean
  link: string
  days: number
  invited: number
  paid: number
  rewardDays: number
}

type Load = { status: 'loading' } | { status: 'ready'; ref: Referral } | { status: 'error' }

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex flex-1 flex-col items-center gap-0.5 rounded-md bg-fill-tertiary px-2 py-3">
      <span className="title-3 font-semibold">{value}</span>
      <span className="caption-1 text-text-secondary">{label}</span>
    </div>
  )
}

export default function Friends() {
  const [load, setLoad] = useState<Load>({ status: 'loading' })
  const [copied, setCopied] = useState(false)

  const fetchRef = useCallback(() => {
    get<Referral>('/m/api/referral').then(
      (ref) => setLoad({ status: 'ready', ref }),
      () => setLoad({ status: 'error' }),
    )
  }, [])
  useEffect(fetchRef, [fetchRef])

  if (load.status === 'loading') return <Loading />
  if (load.status === 'error') {
    return (
      <ErrorBlock
        title="Не получилось загрузить"
        onRetry={() => {
          setLoad({ status: 'loading' })
          fetchRef()
        }}
      />
    )
  }
  const { ref } = load
  const copy = async () => {
    if (await copyText(ref.link)) {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }
  return (
    <div className="pt-4">
      <section className="mx-4 mb-4 flex flex-col gap-3 rounded-lg bg-surface p-4">
        <h2 className="headline">Пригласи друга</h2>
        <p className="subheadline text-text-secondary">
          За каждого друга, который оплатит первую подписку, вы оба получаете по {days(ref.days)}.
        </p>
        <div className="flex gap-2">
          <Stat label="Приглашено" value={ref.invited} />
          <Stat label="Оплатили" value={ref.paid} />
          <Stat label="Получено" value={days(ref.rewardDays)} />
        </div>
        {ref.link && (
          <>
            <p className="footnote break-all text-text-secondary">{ref.link}</p>
            <Button size="md" variant="tinted" onClick={copy}>
              {copied ? 'Скопировано' : 'Скопировать ссылку'}
            </Button>
            <Button size="md" variant="filled" onClick={() => shareLink(ref.link, 'Подключайся к BruhaNet')}>
              Поделиться
            </Button>
          </>
        )}
      </section>
    </div>
  )
}
