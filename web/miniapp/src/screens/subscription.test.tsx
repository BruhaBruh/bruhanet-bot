import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Subscription from './Subscription'

const openBot = vi.fn()
const openExternal = vi.fn()
vi.mock('~/lib/telegram', async (orig) => ({ ...(await orig<typeof import('~/lib/telegram')>()), openBot: (u: string) => openBot(u), openExternal: (u: string) => openExternal(u) }))

const ME = { name: 'Бруха', username: 'bruha', botUsername: 'bruhanet_bot', blocked: false, isAdmin: false }
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
      apps: [{
        name: 'Happ', featured: true, urlScheme: 'happ://add/', needBase64: false,
        steps: [
          { title: 'Установка', text: 'Поставь <script>alert(1)</script> приложение', buttons: [{ text: 'App Store', url: 'https://apps.apple.com/x' }] },
          { title: 'Добавление подписки', text: 'Вставь {{SUBSCRIPTION_LINK}}', buttons: [] },
        ],
      }],
    },
    { id: 'android', title: 'Android', apps: [{ name: 'V2Box', featured: false, urlScheme: 'v2box://install-sub?url=', needBase64: true, steps: [] }] },
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
  ...extra,
})

describe('Subscription screen', () => {
  it('shows the term, traffic and devices and points payments to the bot', async () => {
    stub(base())
    render(<Subscription me={ME} />)
    await settle()
    expect(screen.getByText('12 дней')).toBeTruthy()
    expect(screen.getByText(/12 ГБ из 100 ГБ/)).toBeTruthy()
    expect(screen.getByText('2 из 5')).toBeTruthy()
    expect(screen.getByText(/Оплатить и продлить можно только в боте/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Открыть бота' }))
    expect(openBot).toHaveBeenCalledWith('bruhanet_bot')
  })

  it('asks for the link only when the user presses a button and copies it once', async () => {
    const m = stub(base())
    render(<Subscription me={ME} />)
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
    render(<Subscription me={ME} />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Показать QR' }))
    const img = await screen.findByAltText('QR-код подписки')
    expect((img as HTMLImageElement).src.startsWith('data:image/svg+xml')).toBe(true)
    expect(document.body.innerHTML).not.toContain('SECRET-LINK-TOKEN')
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    await waitFor(() => expect(screen.queryByAltText('QR-код подписки')).toBeNull())
  })

  it('opens the app deep link built from the link, base64 when the app needs it', async () => {
    stub(base())
    render(<Subscription me={ME} />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Добавить в Happ' }))
    await settle()
    expect(openExternal).toHaveBeenLastCalledWith(`happ://add/${LINK}`)
    fireEvent.click(screen.getByRole('radio', { name: 'Android' }))
    fireEvent.click(screen.getByRole('button', { name: 'Добавить в V2Box' }))
    await settle()
    expect(openExternal).toHaveBeenLastCalledWith(`v2box://install-sub?url=${btoa(LINK)}`)
  })

  it('picks the platform of the device and renders panel text as plain text', async () => {
    stub(base())
    render(<Subscription me={ME} />)
    await settle()
    expect(screen.getByRole('radio', { name: 'iOS' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByText(/Поставь <script>alert\(1\)<\/script> приложение/)).toBeTruthy()
    expect(document.querySelector('script')).toBeNull()
    expect(screen.queryByText(/SUBSCRIPTION_LINK/)).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'App Store' }))
    expect(openExternal).toHaveBeenCalledWith('https://apps.apple.com/x')
  })

  it('falls back to a manual hint when the panel has no instructions', async () => {
    stub(base({ '/m/api/instructions': () => json({ platforms: [] }) }))
    render(<Subscription me={ME} />)
    await settle()
    expect(screen.getByText(/добавь её в приложение вручную/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Скопировать ссылку' })).toBeTruthy()
  })

  it.each(['none', 'expired', 'disabled'])('state %s keeps the bot banner and hides the connection block', async (state) => {
    stub(base({ '/m/api/subscription': () => json({ ...ACTIVE, state, daysLeft: 0 }) }))
    render(<Subscription me={ME} />)
    await settle()
    expect(screen.getByText(/Оплатить и продлить можно только в боте/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Скопировать ссылку' })).toBeNull()
  })

  it('tells that the panel is down and retries', async () => {
    let down = true
    stub(base({ '/m/api/subscription': () => (down ? json({ error: 'panel_unavailable' }, 502) : json(ACTIVE)) }))
    render(<Subscription me={ME} />)
    await settle()
    expect(screen.getByText('Панель не отвечает')).toBeTruthy()
    down = false
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await settle()
    expect(screen.getByText('12 дней')).toBeTruthy()
  })
})
