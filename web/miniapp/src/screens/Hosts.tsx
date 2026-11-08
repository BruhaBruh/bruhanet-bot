import { useEffect, useState } from 'react'
import { Icon } from '@/ui/icon'
import { Button } from '@/ui/primitives/button'
import { get } from '~/lib/api'
import { hostLinkFetcher, useLinkActions } from './linkActions'

interface Host { id: string; name: string; protocol: string }

const COLLAPSED = 5

// Ключи подключения: каждый хост отдельно, как на странице подписки. Ссылки в списке нет — только по нажатию.
export function Hosts() {
  const [hosts, setHosts] = useState<Host[]>([])
  const [all, setAll] = useState(false)
  const links = useLinkActions()

  useEffect(() => {
    let stopped = false
    // сбой списка не мешает остальному экрану: карточка просто не показывается
    get<{ items: Host[] }>('/m/api/hosts').then(
      (r) => !stopped && setHosts(r.items),
      () => {},
    )
    return () => {
      stopped = true
    }
  }, [])

  if (hosts.length === 0) return null
  const shown = all ? hosts : hosts.slice(0, COLLAPSED)
  return (
    <section className="mx-4 mb-8 flex flex-col gap-3 rounded-lg bg-surface p-4">
      <div className="flex items-center justify-between">
        <h2 className="headline">Ключи подключения</h2>
        <span className="caption-1 rounded-full bg-accent-tint px-2.5 py-0.5 font-semibold text-accent-strong">{hosts.length}</span>
      </div>
      <ul className="flex flex-col gap-2">
        {shown.map((h) => (
          <li key={h.id} className="flex items-center gap-2 rounded-md bg-fill-tertiary px-3 py-2.5">
            <Icon name="lock" size={16} strokeWidth={2} className="flex-none text-accent-strong" />
            <span className="subheadline min-w-0 flex-1 truncate">{h.name}</span>
            <Button size="sm" variant="plain" aria-label={`Скопировать ключ ${h.name}`} onClick={() => void links.copy(h.id, hostLinkFetcher(h.id))}>
              {links.copiedKey === h.id ? <Icon name="check" size={16} strokeWidth={2.2} /> : <Icon name="copy" size={16} strokeWidth={2} />}
            </Button>
            <Button size="sm" variant="plain" aria-label={`QR ключа ${h.name}`} onClick={() => void links.showQr(`QR-код ключа ${h.name}`, hostLinkFetcher(h.id))}>
              QR
            </Button>
          </li>
        ))}
      </ul>
      {hosts.length > COLLAPSED && (
        <Button size="md" variant="gray" onClick={() => setAll((v) => !v)}>
          {all ? 'Свернуть' : `Показать все (${hosts.length})`}
        </Button>
      )}
      {links.error && <p className="footnote text-destructive-strong">{links.error}</p>}
      {links.qrDialog}
    </section>
  )
}
