import { AppRoot } from '@telegram-apps/telegram-ui'
import { act, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Settings from './Settings'
import Texts from './Texts'

// Редактор подменён: тест читает value и правит через onChange.
const kids = vi.hoisted(() => ({ editor: {} as any }))
vi.mock('../editor/TgEditor', async () => {
  const { forwardRef } = await import('react')
  return {
    TgEditor: forwardRef((p: any, _ref) => {
      kids.editor = p
      return null
    }),
  }
})

type Call = { url: string; method: string; body: any }
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
const wrap = (path: string) =>
  function W({ children }: { children: ReactNode }) {
    return (
      <AppRoot>
        <MemoryRouter initialEntries={[path]}>{children}</MemoryRouter>
      </AppRoot>
    )
  }
const btn = (label: string) => screen.getAllByRole('button').find((b) => b.textContent === label)!
const puts = () => calls.filter((c) => c.method === 'PUT')

const text = (key: string, title: string, body: string) => ({ key, group: 'Меню', title, kind: 'message', default: body, body, overridden: false, vars: [] })

describe('Texts', () => {
  beforeEach(() => {
    replies['GET /api/texts'] = json(200, { items: [text('a', 'Текст A', 'тело A'), text('b', 'Текст B', 'тело B')] })
    replies['PUT /api/texts/b'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
  })

  it('switching keys replaces editor content, unsaved edits do not leak', async () => {
    const { container } = render(<Texts />, { wrapper: wrap('/texts?key=a') })
    await flush()
    expect(kids.editor.value).toBe('тело A')
    act(() => kids.editor.onChange('правка A'))
    await flush()
    expect(container.querySelector('.tg-msg')!.textContent).toBe('правка A')

    fireEvent.click(screen.getByText('Текст B'))
    await flush()
    expect(kids.editor.value).toBe('тело B')
    expect(container.querySelector('.tg-msg')!.textContent).toBe('тело B')
    expect(container.textContent).not.toContain('правка A')
    expect(puts()).toHaveLength(0)
  })

  it('double «Сохранить» sends one PUT', async () => {
    render(<Texts />, { wrapper: wrap('/texts?key=b') })
    await flush()
    act(() => kids.editor.onChange('новое B'))
    await flush()
    fireEvent.click(btn('Сохранить'))
    fireEvent.click(btn('Сохранить'))
    await act(() => new Promise((r) => setTimeout(r, 30)))
    expect(puts()).toEqual([{ url: '/api/texts/b', method: 'PUT', body: { body: 'новое B' } }])
  })
})

describe('Settings', () => {
  const s = { prices: { '1': 150, '3': 400 }, phone: '+7900', bank: 'Банк', comments: ['На кофе'], supportUrl: 'https://t.me/x' }

  it('shows stored problems banner', async () => {
    replies['GET /api/settings'] = json(200, { ...s, usingDefaults: true, problems: ['нет тарифов'] })
    const { container } = render(<Settings />, { wrapper: wrap('/settings') })
    await flush()
    expect(container.textContent).toContain('нет тарифов')
  })

  it('double «Сохранить» sends one PUT with the whole object', async () => {
    replies['GET /api/settings'] = json(200, s)
    replies['PUT /api/settings'] = () => new Promise((r) => setTimeout(() => r(json(204)()), 10))
    render(<Settings />, { wrapper: wrap('/settings') })
    await flush()
    fireEvent.click(btn('Сохранить'))
    fireEvent.click(btn('Сохранить'))
    await act(() => new Promise((r) => setTimeout(r, 30)))
    expect(puts()).toEqual([{ url: '/api/settings', method: 'PUT', body: s }])
    expect(screen.getByText('Сохранено')).toBeTruthy()
  })
})
