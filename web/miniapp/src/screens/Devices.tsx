import { useRef, useState } from 'react'
import { Dialog } from '@/ui/dialog'
import { Button } from '@/ui/primitives/button'
import { ApiError, del } from '~/lib/api'
import { formatDate } from '~/lib/format'
import { useQuery } from '~/lib/query'
import { ErrorBlock, Loading } from './states'

interface Device {
  hwid: string
  platform: string | null
  osVersion: string | null
  deviceModel: string | null
  createdAt: string
}

interface List {
  items: Device[]
  limit: number | null
}

const title = (d: Device) => d.deviceModel || d.platform || 'Устройство'

export default function Devices() {
  const q = useQuery<List>('/m/api/devices')
  const [ask, setAsk] = useState<{ hwid: string | null } | null>(null) // hwid null — удалить все
  const [error, setError] = useState('')
  const busy = useRef(false)

  if (q.isLoading) return <Loading />
  if (!q.data) {
    const down = q.error instanceof ApiError && q.error.status === 502
    return down ? <ErrorBlock title="Панель не отвечает" text="Попробуй чуть позже." onRetry={() => void q.refetch()} /> : <ErrorBlock title="Не получилось загрузить" onRetry={() => void q.refetch()} />
  }

  const { items, limit } = q.data
  const remove = async (hwid: string | null) => {
    setAsk(null)
    if (busy.current) return
    busy.current = true
    setError('')
    try {
      await del(hwid === null ? '/m/api/devices' : `/m/api/devices/${encodeURIComponent(hwid)}`)
    } catch (e) {
      // 404 — устройство уже удалено, список просто приводим к факту
      if (!(e instanceof ApiError && e.status === 404)) {
        setError('Не получилось удалить. Попробуй ещё раз.')
        busy.current = false
        return
      }
    }
    // сразу правим кэш (экран и другие вкладки обновляются без запроса), а в фоне сверяемся с панелью
    q.mutate((old) => ({ limit: old?.limit ?? limit, items: hwid === null ? [] : (old?.items ?? items).filter((d) => d.hwid !== hwid) }))
    void q.refetch()
    busy.current = false
  }

  return (
    <div className="pt-4">
      <section className="mx-4 mb-4 flex flex-col gap-3 rounded-lg bg-surface p-4">
        <div className="flex items-center justify-between">
          <h2 className="headline">Устройства</h2>
          <span className="subheadline text-text-secondary">
            {items.length} из {limit ?? '—'}
          </span>
        </div>
        {items.length === 0 && <p className="subheadline text-text-secondary">Устройств нет</p>}
        <ul className="flex flex-col divide-y divide-separator">
          {items.map((d) => (
            <li key={d.hwid} className="flex items-center justify-between gap-3 py-3">
              <div className="min-w-0">
                <div className="subheadline truncate font-semibold">{title(d)}</div>
                {d.deviceModel && (d.platform || d.osVersion) && (
                  <div className="footnote truncate text-text-secondary">{[d.platform, d.osVersion].filter(Boolean).join(' ')}</div>
                )}
                <div className="footnote text-text-secondary">Добавлено {formatDate(d.createdAt)}</div>
              </div>
              <Button size="sm" variant="destructive" aria-label={`Удалить ${title(d)}`} onClick={() => setAsk({ hwid: d.hwid })}>
                Удалить
              </Button>
            </li>
          ))}
        </ul>
        {error && <p className="footnote text-destructive-strong">{error}</p>}
        {items.length > 0 && (
          <Button size="md" variant="destructive" onClick={() => setAsk({ hwid: null })}>
            Удалить все
          </Button>
        )}
      </section>
      <p className="footnote mx-4 mb-4 text-text-secondary">Лимит и срок подписки при удалении не меняются. Устройство добавится снова при следующем подключении.</p>
      {ask && (
        <Dialog
          title={ask.hwid === null ? 'Удалить все устройства?' : 'Удалить устройство?'}
          message="Приложению на нём придётся подключиться заново."
          actions={[
            { label: 'Отмена', style: 'cancel', onClick: () => setAsk(null) },
            { label: 'Удалить', style: 'destructive', onClick: () => void remove(ask.hwid) },
          ]}
        />
      )}
    </div>
  )
}
