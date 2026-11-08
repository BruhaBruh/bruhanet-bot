import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const openAdmin = vi.fn()
vi.mock('~/lib/telegram', async (orig) => ({ ...(await orig<typeof import('~/lib/telegram')>()), openAdmin: (u: string) => openAdmin(u) }))

const ME = { name: 'Бруха', username: 'bruha', botUsername: 'bruhanet_bot', blocked: false }
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })

// Ответы по путям; всё, чего нет в таблице, — пустой 404, чтобы экраны вкладок не падали в тестах оболочки.
function stubApi(routes: Record<string, () => Response>) {
  const m = vi.fn(async (url: string) => (routes[url] ?? (() => json({}, 404)))())
  vi.stubGlobal('fetch', m)
  return m
}

const referral = (enabled: boolean) => () => json({ enabled })

beforeEach(() => {
  window.Telegram = { WebApp: { initData: 'x', ready: vi.fn(), expand: vi.fn(), close: vi.fn() } as unknown as NonNullable<Window['Telegram']>['WebApp'] }
  window.history.replaceState(null, '', '/')
})

afterEach(() => {
  vi.unstubAllGlobals()
  delete window.Telegram
})

const flush = () => act(async () => {})

describe('App shell', () => {
  it('shows the four tabs and calls ready/expand', async () => {
    stubApi({ '/m/api/me': () => json(ME), '/m/api/referral': referral(true) })
    render(<App />)
    await flush()
    for (const name of ['Подписка', 'Устройства', 'Друзья', 'Промокоды']) {
      expect(screen.getByRole('button', { name })).toBeTruthy()
    }
    expect(window.Telegram?.WebApp?.ready).toHaveBeenCalled()
    expect(window.Telegram?.WebApp?.expand).toHaveBeenCalled()
  })

  it('hides «Друзья» while the referral program is off', async () => {
    stubApi({ '/m/api/me': () => json(ME), '/m/api/referral': referral(false) })
    render(<App />)
    await flush()
    expect(screen.queryByRole('button', { name: 'Друзья' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Промокоды' })).toBeTruthy()
  })

  it('opens the tab named in ?tab= and keeps the url in sync', async () => {
    stubApi({ '/m/api/me': () => json(ME), '/m/api/referral': referral(true) })
    window.history.replaceState(null, '', '/?tab=devices')
    render(<App />)
    await flush()
    expect(screen.getByRole('button', { name: 'Устройства' }).getAttribute('aria-current')).toBe('page')
    fireEvent.click(screen.getByRole('button', { name: 'Промокоды' }))
    expect(screen.getByRole('button', { name: 'Промокоды' }).getAttribute('aria-current')).toBe('page')
    expect(window.location.search).toBe('?tab=promos')
  })

  it('an unknown or hidden tab falls back to the subscription', async () => {
    stubApi({ '/m/api/me': () => json(ME), '/m/api/referral': referral(false) })
    window.history.replaceState(null, '', '/?tab=friends')
    render(<App />)
    await flush()
    expect(screen.getByRole('button', { name: 'Подписка' }).getAttribute('aria-current')).toBe('page')
  })

  it('tells the user to reopen the app when the signature expired', async () => {
    stubApi({ '/m/api/me': () => json({ error: 'session_expired' }, 401), '/m/api/referral': () => json({ error: 'session_expired' }, 401) })
    render(<App />)
    await flush()
    expect(screen.getByText('Сессия закончилась')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(window.Telegram?.WebApp?.close).toHaveBeenCalled()
  })

  it('asks a user the bot has never seen to write /start', async () => {
    stubApi({ '/m/api/me': () => json({ error: 'not_registered' }, 403), '/m/api/referral': () => json({ error: 'not_registered' }, 403) })
    render(<App />)
    await flush()
    expect(screen.getByText(/Сначала напиши боту/)).toBeTruthy()
  })

  it('offers a retry after a server failure', async () => {
    let fail = true
    stubApi({
      '/m/api/me': () => (fail ? json({}, 500) : json(ME)),
      '/m/api/referral': referral(false),
    })
    render(<App />)
    await flush()
    expect(screen.getByText('Не получилось загрузить')).toBeTruthy()
    fail = false
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await flush()
    expect(screen.getByRole('button', { name: 'Подписка' })).toBeTruthy()
  })

  it('returns to a visited tab instantly from the cache and refreshes it in the background', async () => {
    const dev = (model: string) => ({ limit: 5, items: [{ hwid: 'h', platform: 'iOS', osVersion: '18', deviceModel: model, createdAt: '2026-10-01T10:00:00Z' }] })
    let devicesCalls = 0
    let releaseSecond: (r: Response) => void = () => {}
    const second = new Promise<Response>((res) => (releaseSecond = res))
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url === '/m/api/me') return json(ME)
        if (url === '/m/api/referral') return referral(true)()
        if (url === '/m/api/devices') return ++devicesCalls === 1 ? json(dev('Старый')) : second
        return json({ items: [], state: 'none', devices: {} }, 200)
      }),
    )
    window.history.replaceState(null, '', '/?tab=devices')
    render(<App />)
    await flush()
    expect(screen.getByText('Старый')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Промокоды' }))
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Устройства' }))
    // кэш виден сразу, без индикатора загрузки, запрос за свежими данными ещё висит
    expect(screen.getByText('Старый')).toBeTruthy()
    expect(screen.queryByRole('status')).toBeNull()
    expect(devicesCalls).toBe(2)
    await act(async () => releaseSecond(json(dev('Новый'))))
    expect(screen.getByText('Новый')).toBeTruthy()
    expect(screen.queryByText('Старый')).toBeNull()
  })

  describe('admin button', () => {
    const ADMIN = { ...ME, isAdmin: true }

    it('is shown only to the admin', async () => {
      stubApi({ '/m/api/me': () => json(ME), '/m/api/referral': referral(false) })
      const { unmount } = render(<App />)
      await flush()
      expect(screen.queryByRole('button', { name: 'Админка' })).toBeNull()
      unmount()
      stubApi({ '/m/api/me': () => json(ADMIN), '/m/api/referral': referral(false) })
      render(<App />)
      await flush()
      expect(screen.getByRole('button', { name: 'Админка' })).toBeTruthy()
    })

    it('asks the server for a one-time link on tap and opens it', async () => {
      const m = stubApi({
        '/m/api/me': () => json(ADMIN),
        '/m/api/referral': referral(false),
        '/m/api/admin/login-link': () => json({ url: 'https://admin.example.test/auth?token=T' }),
      })
      render(<App />)
      await flush()
      const calls = () => m.mock.calls.filter(([u]) => u === '/m/api/admin/login-link').length
      expect(calls()).toBe(0)
      fireEvent.click(screen.getByRole('button', { name: 'Админка' }))
      await flush()
      expect(calls()).toBe(1)
      expect(openAdmin).toHaveBeenCalledWith('https://admin.example.test/auth?token=T')
    })

    it('explains a stale signature and a failure', async () => {
      let status = 401
      stubApi({
        '/m/api/me': () => json(ADMIN),
        '/m/api/referral': referral(false),
        '/m/api/admin/login-link': () => json({ error: status === 401 ? 'session_expired' : 'x' }, status),
      })
      render(<App />)
      await flush()
      fireEvent.click(screen.getByRole('button', { name: 'Админка' }))
      await flush()
      expect(screen.getByText('Открой кабинет заново из бота')).toBeTruthy()
      status = 500
      fireEvent.click(screen.getByRole('button', { name: 'Админка' }))
      await flush()
      expect(screen.getByText('Не получилось открыть админку')).toBeTruthy()
    })
  })
})
