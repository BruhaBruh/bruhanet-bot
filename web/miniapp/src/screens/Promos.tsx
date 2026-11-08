import { useRef, useState } from 'react'
import { Input } from '@/ui/input'
import { Button } from '@/ui/primitives/button'
import { ApiError, post } from '~/lib/api'
import { days, formatDate } from '~/lib/format'
import { useQuery } from '~/lib/query'
import { ErrorBlock, Loading } from './states'

interface Used {
  code: string
  kind: 'days' | 'percent' | 'rub'
  value: number
  activatedAt: string
}

const gave = (p: Used) => (p.kind === 'days' ? `+${days(p.value)}` : p.kind === 'percent' ? `скидка ${p.value}%` : `скидка ${p.value} ₽`)

export default function Promos() {
  const q = useQuery<{ items: Used[] }>('/m/api/promos')
  const [code, setCode] = useState('')
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null)
  const [pending, setPending] = useState(false)
  const busy = useRef(false)

  const activate = async () => {
    const value = code.trim()
    if (!value || busy.current) return
    busy.current = true
    setPending(true)
    setResult(null)
    try {
      const r = await post<{ addedDays: number; expireAt: string }>('/m/api/promos/activate', { code: value })
      setResult({ ok: true, text: `Добавлено ${days(r.addedDays)}. Подписка до ${formatDate(r.expireAt)}.` })
      setCode('')
      void q.refetch()
    } catch (e) {
      setResult({ ok: false, text: (e instanceof ApiError && e.serverMessage) || 'Не получилось. Попробуй ещё раз.' })
    } finally {
      busy.current = false
      setPending(false)
    }
  }

  return (
    <div className="pt-4">
      <section className="mx-4 mb-4 flex flex-col gap-3 rounded-lg bg-surface p-4 [--field-bg:var(--page-bg)]">
        <h2 className="headline">Промокод</h2>
        <Input placeholder="Промокод" value={code} autoCapitalize="characters" autoComplete="off" onChange={(e) => setCode(e.target.value)} />
        <Button size="md" disabled={!code.trim() || pending} onClick={() => void activate()}>
          Активировать
        </Button>
        {result && <p className={`footnote ${result.ok ? 'text-success-strong' : 'text-destructive-strong'}`}>{result.text}</p>}
        <p className="footnote text-text-secondary">Здесь работают коды на бонусные дни. Скидочные коды вводятся при оплате в боте.</p>
      </section>
      {q.isLoading && <Loading />}
      {!q.data && q.error !== undefined && <ErrorBlock title="Не получилось загрузить список" onRetry={() => void q.refetch()} />}
      {q.data && q.data.items.length > 0 && (
        <section className="mx-4 mb-4 flex flex-col gap-1 rounded-lg bg-surface p-4">
          <h2 className="headline pb-1">Использованные</h2>
          <ul className="flex flex-col divide-y divide-separator">
            {q.data.items.map((p) => (
              <li key={p.code + p.activatedAt} className="flex items-center justify-between py-2.5">
                <div>
                  <div className="subheadline font-semibold">{p.code}</div>
                  <div className="footnote text-text-secondary">{formatDate(p.activatedAt)}</div>
                </div>
                <span className="subheadline text-text-secondary">{gave(p)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
