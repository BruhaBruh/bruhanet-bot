import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Subscription from './Subscription'

const openExternal = vi.fn()
vi.mock('~/lib/telegram', async (orig) => ({ ...(await orig<typeof import('~/lib/telegram')>()), openExternal: (u: string) => openExternal(u) }))

const LINK = 'https://sub.example.test/SECRET-LINK-TOKEN'
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })

const ACTIVE = {
  state: 'active', expireAt: '2026-11-20T12:00:00Z', daysLeft: 12, trafficUsedBytes: 12 * 1024 ** 3, trafficLimitBytes: 100 * 1024 ** 3,
  devices: { used: 2, limit: 5 },
}
const INSTR = {
  platforms: [
    {
      id: 'ios', title: 'iOS',
      apps: [
        {
          name: 'Happ', featured: true,
          blocks: [
            { title: 'Установка приложения', text: 'Поставь <script>alert(1)</script> приложение', icon: 'download', tone: 'cyan', buttons: [{ text: 'App Store', url: 'https://apps.apple.com/x', kind: 'link' }] },
            { title: 'Добавление подписки', text: 'Вставь {{SUBSCRIPTION_LINK}}', icon: 'cloud', tone: 'teal', buttons: [{ text: 'Добавить подписку', url: 'happ://add/{{SUBSCRIPTION_LINK}}', kind: 'subscription' }] },
            { title: 'Подключение и использование', text: '', icon: 'check', tone: 'teal', buttons: [] },
          ],
        },
        { name: 'Stash', featured: false, blocks: [{ title: 'Установка Stash', text: 'Stash', icon: 'download', tone: 'cyan', buttons: [] }] },
      ],
    },
    {
      id: 'android', title: 'Android',
      apps: [{ name: 'V2Box', featured: false, blocks: [{ title: 'Добавление', text: '', icon: 'cloud', tone: 'teal', buttons: [{ text: 'Добавить подписку', url: 'v2rayng://install-config?url={{SUBSCRIPTION_LINK}}', kind: 'subscription' }] }] }],
    },
  ],
}

function stub(routes: Record<string, () => Response>) {
  const m = vi.fn(async (url: string) => (routes[url] ?? (() => json({}, 404)))())
  vi.stubGlobal('fetch', m)
  return m
}
const linkCalls = (m: ReturnType<typeof stub>) => m.mock.calls.filter(([u]) => u === '/m/api/subscription-link').length
const settle = () => act(async () => {})

const writeText = vi.fn(async () => {})
beforeEach(() => {
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  Object.defineProperty(navigator, 'userAgent', { value: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)', configurable: true })
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

const base = (extra: Record<string, () => Response> = {}) => ({
  '/m/api/subscription': () => json(ACTIVE),
  '/m/api/instructions': () => json(INSTR),
  '/m/api/subscription-link': () => json({ url: LINK }),
  '/m/api/hosts': () => json({ items: [] }),
  ...extra,
})

describe('Subscription screen', () => {
  it('shows the term, traffic and devices and explains that payment lives in the bot', async () => {
    stub(base())
    render(<Subscription />)
    await settle()
    expect(screen.getByText('12 д.')).toBeTruthy()
    expect(screen.getByText(/12 ГБ из 100 ГБ/)).toBeTruthy()
    expect(screen.getByText('2 из 5')).toBeTruthy()
    expect(screen.getByText(/Оплатить и продлить можно только в боте/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Открыть бота' })).toBeNull()
  })

  it('asks for the link only when the user presses a button and copies it once', async () => {
    const m = stub(base())
    render(<Subscription />)
    await settle()
    expect(linkCalls(m)).toBe(0)
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать ссылку' }))
    await settle()
    expect(linkCalls(m)).toBe(1)
    expect(writeText).toHaveBeenCalledWith(LINK)
    expect(document.body.innerHTML).not.toContain('SECRET-LINK-TOKEN')
  })

  it('draws the QR from the fetched link and forgets it on close', async () => {
    stub(base())
    render(<Subscription />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Показать QR' }))
    const img = await screen.findByAltText('QR-код подписки')
    expect((img as HTMLImageElement).src.startsWith('data:image/svg+xml')).toBe(true)
    expect(document.body.innerHTML).not.toContain('SECRET-LINK-TOKEN')
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    await waitFor(() => expect(screen.queryByAltText('QR-код подписки')).toBeNull())
  })

  it('opens the app deep link with the link substituted, fetching it only after the tap', async () => {
    const m = stub(base())
    render(<Subscription />)
    await settle()
    expect(linkCalls(m)).toBe(0)
    fireEvent.click(screen.getByRole('button', { name: 'Добавить подписку' }))
    await settle()
    expect(linkCalls(m)).toBe(1)
    expect(openExternal).toHaveBeenLastCalledWith(`happ://add/${LINK}`)
    fireEvent.click(screen.getByRole('radio', { name: 'Android' }))
    fireEvent.click(screen.getByRole('button', { name: 'Добавить подписку' }))
    await settle()
    expect(openExternal).toHaveBeenLastCalledWith(`v2rayng://install-config?url=${LINK}`)
  })

  it('renders the blocks of the device platform as plain text; external buttons need no link', async () => {
    const m = stub(base())
    render(<Subscription />)
    await settle()
    expect(screen.getByRole('radio', { name: 'iOS' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByText('Установка приложения')).toBeTruthy()
    expect(screen.getByText(/Поставь <script>alert\(1\)<\/script> приложение/)).toBeTruthy()
    expect(document.querySelector('script')).toBeNull()
    expect(screen.getByText('Вставь ссылку подписки')).toBeTruthy()
    expect(screen.getByText('Подключение и использование')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'App Store' }))
    expect(openExternal).toHaveBeenCalledWith('https://apps.apple.com/x')
    expect(linkCalls(m)).toBe(0)
  })

  it('switches between the apps of a platform', async () => {
    stub(base())
    render(<Subscription />)
    await settle()
    expect(screen.getByText('Установка приложения')).toBeTruthy()
    fireEvent.click(screen.getByRole('radio', { name: 'Stash' }))
    expect(screen.queryByText('Установка приложения')).toBeNull()
    expect(screen.getByText('Установка Stash')).toBeTruthy()
  })

  it('falls back to a manual hint when the panel has no instructions', async () => {
    stub(base({ '/m/api/instructions': () => json({ platforms: [] }) }))
    render(<Subscription />)
    await settle()
    expect(screen.getByText(/добавь её в приложение вручную/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Скопировать ссылку' })).toBeTruthy()
  })

  it.each(['none', 'expired', 'disabled'])('state %s keeps the bot banner and hides the connection block', async (state) => {
    stub(base({ '/m/api/subscription': () => json({ ...ACTIVE, state, daysLeft: 0 }) }))
    render(<Subscription />)
    await settle()
    expect(screen.getByText(/Оплатить и продлить можно только в боте/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Скопировать ссылку' })).toBeNull()
  })

  it('tells that the panel is down and retries', async () => {
    let down = true
    stub(base({ '/m/api/subscription': () => (down ? json({ error: 'panel_unavailable' }, 502) : json(ACTIVE)) }))
    render(<Subscription />)
    await settle()
    expect(screen.getByText('Панель не отвечает')).toBeTruthy()
    down = false
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await settle()
    expect(screen.getByText('12 д.')).toBeTruthy()
  })
})

describe('Connection keys (hosts)', () => {
  const HOSTLINK = 'vless://11111111-2222-4333-8444-555555555555@h.example:443#HOST-SECRET'
  const item = (i: number, name = `Хост ${i}`) => ({ id: String(i).padStart(16, 'a'), name, protocol: 'vless' })
  const HOSTS = { items: [{ id: 'aaaaaaaaaaaaaaaa', name: '⚡️ Авто', protocol: 'vless' }, { id: 'bbbbbbbbbbbbbbbb', name: '🇩🇪 Hysteria 2', protocol: 'hysteria2' }] }
  const hostCalls = (m: ReturnType<typeof stub>) => (m.mock.calls as unknown as [string, RequestInit | undefined][]).filter(([u]) => u === '/m/api/hosts/link')

  const withHosts = (hosts: unknown) => base({ '/m/api/hosts': () => json(hosts), '/m/api/hosts/link': () => json({ url: HOSTLINK }) })

  it('lists the hosts with a counter and asks for no link', async () => {
    const m = stub(withHosts(HOSTS))
    render(<Subscription />)
    await settle()
    expect(screen.getByText('Ключи подключения')).toBeTruthy()
    expect(screen.getByText('⚡️ Авто')).toBeTruthy()
    expect(screen.getByText('🇩🇪 Hysteria 2')).toBeTruthy()
    expect(screen.getByText('2')).toBeTruthy()
    expect(hostCalls(m)).toHaveLength(0)
  })

  it('copies one key on demand and keeps the link out of the DOM', async () => {
    const m = stub(withHosts(HOSTS))
    render(<Subscription />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать ключ ⚡️ Авто' }))
    await settle()
    expect(hostCalls(m)).toHaveLength(1)
    expect(JSON.parse(hostCalls(m)[0][1]?.body as string)).toEqual({ id: 'aaaaaaaaaaaaaaaa' })
    expect(writeText).toHaveBeenCalledWith(HOSTLINK)
    expect(document.body.innerHTML).not.toContain('HOST-SECRET')
  })

  it('shows a QR of one key', async () => {
    stub(withHosts(HOSTS))
    render(<Subscription />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'QR ключа 🇩🇪 Hysteria 2' }))
    const img = await screen.findByAltText('QR-код ключа 🇩🇪 Hysteria 2')
    expect((img as HTMLImageElement).src.startsWith('data:image/svg+xml')).toBe(true)
    expect(document.body.innerHTML).not.toContain('HOST-SECRET')
  })

  it('collapses a long list to five and expands it on request', async () => {
    stub(withHosts({ items: [1, 2, 3, 4, 5, 6, 7].map((i) => item(i)) }))
    render(<Subscription />)
    await settle()
    expect(screen.queryByText('Хост 6')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Показать все (7)' }))
    expect(screen.getByText('Хост 7')).toBeTruthy()
  })

  it('hides the card when there are no keys or the request fails', async () => {
    stub(withHosts({ items: [] }))
    const { unmount } = render(<Subscription />)
    await settle()
    expect(screen.queryByText('Ключи подключения')).toBeNull()
    unmount()
    stub(base({ '/m/api/hosts': () => json({ error: 'panel_unavailable' }, 502) }))
    render(<Subscription />)
    await settle()
    expect(screen.queryByText('Ключи подключения')).toBeNull()
    expect(screen.getByText('12 д.')).toBeTruthy()
  })
})
