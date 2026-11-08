import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

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
})
