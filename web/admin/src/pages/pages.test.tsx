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
