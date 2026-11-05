import { AppRoot } from '@telegram-apps/telegram-ui'
import { act, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Settings from './Settings'

type Call = { url: string; method: string; body: unknown }
let calls: Call[]
let replies: Record<string, () => Response | Promise<Response>>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  calls = []
  replies = {}
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return (replies[`${method} ${url}`] ?? json(500))()
  })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const wrapper = ({ children }: { children: ReactNode }) => (
  <AppRoot>
    <MemoryRouter>{children}</MemoryRouter>
  </AppRoot>
)
const btn = (label: string) => screen.getAllByRole('button').find((b) => b.textContent === label)!

describe('Settings', () => {
  const s = { prices: { '1': 150, '3': 400 }, phone: '+7900', bank: 'Банк', comments: ['На кофе'], supportUrl: 'https://t.me/x' }

  it('shows stored problems banner', async () => {
    replies['GET /api/settings'] = json(200, { ...s, usingDefaults: true, problems: ['нет тарифов'] })
    const { container } = render(<Settings />, { wrapper })
    await flush()
    expect(container.textContent).toContain('нет тарифов')
  })

  it('double «Сохранить» sends one PUT with the whole object', async () => {
    replies['GET /api/settings'] = json(200, s)
    replies['PUT /api/settings'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
    render(<Settings />, { wrapper })
    await flush()
    fireEvent.click(btn('Сохранить'))
    fireEvent.click(btn('Сохранить'))
    await act(() => new Promise((r) => setTimeout(r, 30)))
    expect(calls.filter((c) => c.method === 'PUT')).toEqual([{ url: '/api/settings', method: 'PUT', body: s }])
    expect(screen.getByText('Сохранено')).toBeTruthy()
  })
})
