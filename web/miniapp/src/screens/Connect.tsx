import QRCode from 'qrcode'
import { useEffect, useState } from 'react'
import { copyText } from '@/lib/clipboard'
import { Dialog } from '@/ui/dialog'
import { Button } from '@/ui/primitives/button'
import { SegmentedControl } from '@/ui/segmented-control'
import { get, post } from '~/lib/api'
import { openExternal } from '~/lib/telegram'

interface Step { title: string; text: string; buttons: { text: string; url: string }[] }
interface App { name: string; featured: boolean; urlScheme: string; needBase64: boolean; steps: Step[] }
interface Platform { id: string; title: string; apps: App[] }

const PLACEHOLDER = /\{\{\s*SUBSCRIPTION_LINK\s*\}\}/g

// Платформа устройства, с которого открыли кабинет; иначе первая из конфига.
function detect(platforms: Platform[]): string {
  const ua = navigator.userAgent
  const want = /iPhone|iPad|iPod/.test(ua) ? 'ios' : /Android/.test(ua) ? 'android' : /Windows/.test(ua) ? 'windows' : /Mac/.test(ua) ? 'macos' : /Linux/.test(ua) ? 'linux' : ''
  return platforms.find((p) => p.id === want)?.id ?? platforms[0]?.id ?? ''
}

// Ссылку подписки берём отдельным запросом по нажатию и никуда не сохраняем: ни в состояние экрана, ни в хранилище.
const fetchLink = async () => (await post<{ url: string }>('/m/api/subscription-link')).url

export function Connect() {
  const [platforms, setPlatforms] = useState<Platform[] | null>(null)
  const [platformId, setPlatformId] = useState('')
  const [appName, setAppName] = useState('')
  const [qr, setQr] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let stopped = false
    get<{ platforms: Platform[] }>('/m/api/instructions').then(
      (r) => {
        if (stopped) return
        setPlatforms(r.platforms)
        setPlatformId(detect(r.platforms))
      },
      () => !stopped && setPlatforms([]),
    )
    return () => {
      stopped = true
    }
  }, [])

  const withLink = async (fn: (link: string) => void | Promise<void>) => {
    setError('')
    try {
      await fn(await fetchLink())
    } catch {
      setError('Не получилось получить ссылку. Попробуй ещё раз.')
    }
  }

  const copy = () =>
    withLink(async (link) => {
      if (await copyText(link)) {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      } else setError('Не получилось скопировать.')
    })

  const showQr = () =>
    withLink(async (link) => {
      const svg = await QRCode.toString(link, { type: 'svg', margin: 1 })
      setQr(`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`)
    })

  const platform = platforms?.find((p) => p.id === platformId)
  const app = platform?.apps.find((a) => a.name === appName) ?? platform?.apps[0]

  return (
    <section className="mx-4 mb-8 flex flex-col gap-4 rounded-lg bg-surface p-4">
      <h2 className="headline">Подключение</h2>

      {platforms && platforms.length > 0 && (
        <>
          <SegmentedControl
            options={platforms.map((p) => ({ value: p.id, label: p.title }))}
            value={platformId}
            onChange={(id) => {
              setPlatformId(id)
              setAppName('')
            }}
          />
          {platform && platform.apps.length > 1 && (
            <SegmentedControl options={platform.apps.map((a) => a.name)} value={app?.name} onChange={setAppName} />
          )}
          {app && (
            <ol className="flex flex-col gap-4">
              {app.steps.map((s, i) => (
                <li key={s.title} className="flex flex-col gap-2">
                  <h3 className="subheadline font-semibold">
                    {i + 1}. {s.title}
                  </h3>
                  {s.text && <p className="subheadline whitespace-pre-line text-text-secondary">{s.text.replace(PLACEHOLDER, 'ссылку подписки')}</p>}
                  {s.buttons.map((b) => (
                    <Button key={b.url} size="md" variant="gray" onClick={() => openExternal(b.url)}>
                      {b.text}
                    </Button>
                  ))}
                </li>
              ))}
            </ol>
          )}
          {app?.urlScheme && (
            <Button
              size="lg"
              onClick={() => withLink((link) => openExternal(app.urlScheme + (app.needBase64 ? btoa(link) : link)))}
            >
              Добавить в {app.name}
            </Button>
          )}
        </>
      )}
      {platforms && platforms.length === 0 && (
        <p className="subheadline text-text-secondary">Скопируй ссылку подписки и добавь её в приложение вручную.</p>
      )}

      <div className="flex flex-col gap-2">
        <Button size="md" variant="tinted" onClick={copy}>
          {copied ? 'Скопировано' : 'Скопировать ссылку'}
        </Button>
        <Button size="md" variant="gray" onClick={showQr}>
          Показать QR
        </Button>
        {error && <p className="footnote text-destructive-strong">{error}</p>}
      </div>

      {qr && (
        <Dialog title="QR-код подписки" message="Отсканируй его в приложении" actions={[{ label: 'Закрыть', style: 'cancel', onClick: () => setQr(null) }]}>
          <img src={qr} alt="QR-код подписки" className="mx-auto my-3 size-56 rounded-md bg-white" />
        </Dialog>
      )}
    </section>
  )
}
