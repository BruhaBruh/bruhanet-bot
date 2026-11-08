import QRCode from 'qrcode'
import { useState, type ReactNode } from 'react'
import { copyText } from '@/lib/clipboard'
import { Dialog } from '@/ui/dialog'
import { post } from '~/lib/api'
import { openExternal } from '~/lib/telegram'

// Ссылки (подписки и ключей подключения) — доступ к VPN. Берём их отдельным запросом по нажатию и никуда не сохраняем:
// ни в состояние экрана, ни в хранилище. В состоянии остаётся только картинка QR, пока открыт диалог.
export const fetchSubscriptionLink = async () => (await post<{ url: string }>('/m/api/subscription-link')).url
export const hostLinkFetcher = (id: string) => async () => (await post<{ url: string }>('/m/api/hosts/link', { id })).url

export function useLinkActions(): {
  copiedKey: string | null
  error: string
  copy(key: string, fetcher: () => Promise<string>): Promise<void>
  showQr(title: string, fetcher: () => Promise<string>): Promise<void>
  openWith(fetcher: () => Promise<string>, build: (link: string) => string): Promise<void>
  qrDialog: ReactNode
} {
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [qr, setQr] = useState<{ title: string; src: string } | null>(null)

  const run = async (fetcher: () => Promise<string>, then: (link: string) => void | Promise<void>) => {
    setError('')
    try {
      await then(await fetcher())
    } catch {
      setError('Не получилось получить ссылку. Попробуй ещё раз.')
    }
  }

  return {
    copiedKey,
    error,
    copy: (key, fetcher) =>
      run(fetcher, async (link) => {
        if (await copyText(link)) {
          setCopiedKey(key)
          setTimeout(() => setCopiedKey((k) => (k === key ? null : k)), 2000)
        } else setError('Не получилось скопировать.')
      }),
    showQr: (title, fetcher) =>
      run(fetcher, async (link) => {
        const svg = await QRCode.toString(link, { type: 'svg', margin: 1 })
        setQr({ title, src: `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}` })
      }),
    openWith: (fetcher, build) => run(fetcher, (link) => openExternal(build(link))),
    qrDialog: qr && (
      <Dialog title={qr.title} message="Отсканируй его в приложении" actions={[{ label: 'Закрыть', style: 'cancel', onClick: () => setQr(null) }]}>
        <img src={qr.src} alt={qr.title} className="mx-auto my-3 size-56 rounded-md bg-white" />
      </Dialog>
    ),
  }
}
