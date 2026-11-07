import { useEffect, useState } from 'react'
import { Button, Tip, toast } from '@/ui'
import { ApiError, get, post } from '../api'
import { formatCount, formatShortDate } from '../format'

const LABEL = 'Синхронизировать с панелью'

export function SyncButton({ onSynced }: { onSynced: () => void }) {
  const [busy, setBusy] = useState(false)
  const [last, setLast] = useState<string | null>(null)

  useEffect(() => {
    let stale = false
    get<{ lastSyncAt: string | null }>('/api/sync').then(
      (r) => {
        if (!stale) setLast(r.lastSyncAt)
      },
      () => {}, // подсказка в title, без неё кнопка работает
    )
    return () => {
      stale = true
    }
  }, [])

  async function sync() {
    if (busy) return
    setBusy(true)
    try {
      const r = await post<{ total: number; finishedAt: string }>('/api/sync')
      setLast(r.finishedAt)
      toast(`Синхронизировано: ${formatCount(r.total)}`, { icon: 'check' })
      onSynced()
    } catch (e) {
      toast((e instanceof ApiError && e.body.message) || 'Не удалось синхронизировать')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Tip label={last ? `${LABEL}. Последняя: ${formatShortDate(last)}` : LABEL}>
      <Button variant="gray" icon="refresh" aria-label={LABEL} loading={busy} onClick={() => void sync()} />
    </Tip>
  )
}
