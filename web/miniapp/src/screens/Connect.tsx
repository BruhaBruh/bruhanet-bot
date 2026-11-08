import { useState } from 'react'
import { cn } from '@/lib/utils'
import { Icon, type IconName } from '@/ui/icon'
import { Button } from '@/ui/primitives/button'
import { SegmentedControl } from '@/ui/segmented-control'
import { useQuery } from '~/lib/query'
import { openExternal } from '~/lib/telegram'
import { fetchSubscriptionLink, useLinkActions } from './linkActions'

interface Btn { text: string; url: string; kind: 'link' | 'subscription' }
interface Block { title: string; text: string; icon: string; tone: string; buttons: Btn[] }
interface App { name: string; featured: boolean; blocks: Block[] }
interface Platform { id: string; title: string; apps: App[] }

const PLACEHOLDER = /\{\{\s*SUBSCRIPTION_LINK\s*\}\}/g

const ICONS: Record<string, IconName> = { download: 'arrow-down', cloud: 'plus', gear: 'gear', check: 'check' }
const TONES: Record<string, string> = {
  cyan: 'bg-accent-tint text-accent-strong',
  teal: 'bg-success-tint text-success-strong',
  red: 'bg-destructive-tint text-destructive-strong',
  violet: 'bg-warning-tint text-warning-strong',
}

// Платформа устройства, с которого открыли кабинет; иначе первая из конфига.
function detect(platforms: Platform[]): string {
  const ua = navigator.userAgent
  const want = /iPhone|iPad|iPod/.test(ua) ? 'ios' : /Android/.test(ua) ? 'android' : /Windows/.test(ua) ? 'windows' : /Mac/.test(ua) ? 'macos' : /Linux/.test(ua) ? 'linux' : ''
  return platforms.find((p) => p.id === want)?.id ?? platforms[0]?.id ?? ''
}

export function Connect() {
  const q = useQuery<{ platforms: Platform[] }>('/m/api/instructions')
  const [picked, setPicked] = useState('')
  const [appName, setAppName] = useState('')
  const links = useLinkActions()

  // сбой инструкций не страшен: остаётся ручное добавление по ссылке
  const platforms = q.data?.platforms ?? (q.error ? [] : null)
  const platformId = picked || (platforms ? detect(platforms) : '')

  const platform = platforms?.find((p) => p.id === platformId)
  const app = platform?.apps.find((a) => a.name === appName) ?? platform?.apps[0]

  return (
    <section className="mx-4 mb-4 flex flex-col gap-4 rounded-lg bg-surface p-4">
      <h2 className="headline">Установка</h2>

      {platforms && platforms.length > 0 && (
        <>
          <SegmentedControl
            options={platforms.map((p) => ({ value: p.id, label: p.title }))}
            value={platformId}
            onChange={(id) => {
              setPicked(id)
              setAppName('')
            }}
          />
          {platform && platform.apps.length > 1 && <SegmentedControl options={platform.apps.map((a) => a.name)} value={app?.name} onChange={setAppName} />}
          {app && (
            <ol className="relative flex flex-col gap-5">
              {app.blocks.map((b, i) => (
                <li key={`${b.title}-${i}`} className="relative flex gap-3">
                  {i < app.blocks.length - 1 && <span aria-hidden className="absolute top-9 bottom-[-1.25rem] left-4.5 w-px -translate-x-1/2 bg-separator" />}
                  <span className={cn('relative flex size-9 flex-none items-center justify-center rounded-full', TONES[b.tone] ?? TONES.cyan)}>
                    <Icon name={ICONS[b.icon] ?? 'check'} size={18} strokeWidth={2} />
                  </span>
                  <div className="flex min-w-0 flex-1 flex-col gap-1.5 pt-1">
                    {b.title && <h3 className="subheadline font-semibold">{b.title}</h3>}
                    {b.text && <p className="subheadline whitespace-pre-line text-text-secondary">{b.text.replace(PLACEHOLDER, 'ссылку подписки')}</p>}
                    {b.buttons.length > 0 && (
                      <div className="flex flex-wrap gap-2 pt-1">
                        {b.buttons.map((btn) =>
                          btn.kind === 'link' ? (
                            <Button key={btn.url} size="sm" variant="tinted" onClick={() => openExternal(btn.url)}>
                              <Icon name="link" size={14} strokeWidth={2} />
                              {btn.text}
                            </Button>
                          ) : (
                            <Button key={btn.url} size="sm" variant="filled" onClick={() => void links.openWith(fetchSubscriptionLink, (link) => btn.url.replace(PLACEHOLDER, link))}>
                              <Icon name="plus" size={14} strokeWidth={2} />
                              {btn.text}
                            </Button>
                          ),
                        )}
                      </div>
                    )}
                  </div>
                </li>
              ))}
            </ol>
          )}
        </>
      )}
      {platforms && platforms.length === 0 && <p className="subheadline text-text-secondary">Скопируй ссылку подписки и добавь её в приложение вручную.</p>}

      <div className="flex flex-col gap-2">
        <Button size="md" variant="tinted" onClick={() => void links.copy('subscription', fetchSubscriptionLink)}>
          {links.copiedKey === 'subscription' ? 'Скопировано' : 'Скопировать ссылку'}
        </Button>
        <Button size="md" variant="gray" onClick={() => void links.showQr('QR-код подписки', fetchSubscriptionLink)}>
          Показать QR
        </Button>
        {links.error && <p className="footnote text-destructive-strong">{links.error}</p>}
      </div>
      {links.qrDialog}
    </section>
  )
}
