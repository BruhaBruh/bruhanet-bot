import { act, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'

afterEach(() => vi.unstubAllGlobals())

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

// ответы по префиксу пути
function stub(routes: Record<string, () => Response>) {
  vi.stubGlobal('fetch', async (url: string) => {
    const k = Object.keys(routes).find((p) => url.startsWith(p))
    return k ? routes[k]() : json(500, {})
  })
}

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

const renderAt = async (path: string) => {
  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
  await flush()
}

const user = (id: number, username: string, confirmed: number) => ({ telegramId: id, username, createdAt: '2026-09-10T10:00:00Z', confirmed, confirmedSum: confirmed * 300 })

describe('Users', () => {
  it('shows count and pluralizes payments', async () => {
    stub({ '/api/users?': () => json(200, { items: [user(1, 'ann', 1), user(2, '', 2)] }) })
    await renderAt('/users')
    expect(screen.getByText('Найдено: 2')).toBeTruthy()
    expect(screen.getByText('1 оплата')).toBeTruthy()
    expect(screen.getByText('2 оплаты')).toBeTruthy()
    expect(screen.getByText('@ann')).toBeTruthy()
    expect(screen.getByText('1 · с сентября 2026')).toBeTruthy()
    expect(screen.getByText('@ann').closest('a')?.getAttribute('href')).toBe('/users/1')
  })
})

describe('UserCard', () => {
  it('says user not found on 404', async () => {
    stub({ '/api/users/7': () => json(404, { error: 'not_found' }) })
    await renderAt('/users/7')
    expect(screen.getByText('Юзер не найден')).toBeTruthy()
  })

  it('shows panel status in words', async () => {
    stub({
      '/api/users/7': () =>
        json(200, {
          user: user(7, 'bob', 0),
          stats: { confirmed: 0, confirmedSum: 0, unpaidAdvances: 0, lastConfirmedAt: null, firstConfirmedAt: null, hasHistory: false },
          payments: [],
          panel: { expireAt: null, status: 'not_found', error: null },
          messages: [],
        }),
    })
    await renderAt('/users/7')
    expect(screen.getByText('нет в панели')).toBeTruthy()
    expect(screen.getByText('Платежей нет')).toBeTruthy()
    expect(screen.getByText('Сообщений нет')).toBeTruthy()
  })
})

describe('Payments', () => {
  it('shows revenue and payments with user', async () => {
    stub({
      '/api/payments?': () =>
        json(200, {
          items: [{ id: 1, telegramId: 5, username: 'eve', month: 3, price: 900, status: 'confirmed', createdAt: '2026-11-01T10:00:00Z' }],
          total: 1,
          revenue: 900,
        }),
    })
    await renderAt('/payments')
    expect(screen.getByText('всего платежей: 1')).toBeTruthy()
    expect(screen.getByText('@eve · 3 мес').closest('a')?.getAttribute('href')).toBe('/users/5')
  })
})
