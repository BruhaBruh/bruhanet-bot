import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { get } from '../api'
import { App } from '../App'

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })

function stubFetch(pending = 0, me?: object) {
  const fetchMock = vi.fn(async (url: string) =>
    url === '/api/me' ? (me ? json(200, me) : json(500, {})) : url.startsWith('/api/payments') ? json(200, { items: [], total: pending, revenue: 0 }) : url.startsWith('/api/users?') || url === '/api/logout' ? json(200, { items: [] }) : json(500, {}),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

const renderAt = async (path: string) => {
  const view = render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
  await flush()
  return view
}

const sidebar = () => within(document.querySelector('aside')!)
const activeLabel = () => sidebar().getByRole('button', { current: 'page' }).textContent

beforeEach(() => localStorage.clear())
afterEach(() => vi.unstubAllGlobals())

describe('AppShell', () => {
  it('lists sections in macro order with groups', async () => {
    stubFetch()
    await renderAt('/')
    expect(document.querySelector('aside nav')!.textContent).toBe(
      'ОбзорМаркетингРассылкиПромокодыПодаркиБотРедактор сообщенийСхема ботаДанныеПользователиПлатежиСистемаНастройки',
    )
  })

  it('marks the section active on nested routes', async () => {
    stubFetch()
    await renderAt('/users/5')
    expect(activeLabel()).toBe('Пользователи')
    expect(document.title).toBe('Пользователи — BruhaNet')
  })

  it('titles the section in document and header', async () => {
    stubFetch()
    await renderAt('/settings')
    expect(document.title).toBe('Настройки — BruhaNet')
    expect(screen.getByRole('heading', { level: 1, name: 'Настройки' })).toBeTruthy()
  })

  it('names create pages in the header, section in the tab', async () => {
    stubFetch()
    await renderAt('/promos/new')
    expect(screen.getByRole('heading', { level: 1, name: 'Новый промокод' })).toBeTruthy()
    expect(document.title).toBe('Промокоды — BruhaNet')
    expect(activeLabel()).toBe('Промокоды')
  })

  it('sends unknown paths to the overview', async () => {
    stubFetch()
    await renderAt('/nope')
    expect(activeLabel()).toBe('Обзор')
  })

  it('remembers the collapsed sidebar', async () => {
    stubFetch()
    const first = await renderAt('/')
    fireEvent.click(sidebar().getByRole('button', { name: 'Свернуть меню' }))
    expect(sidebar().getByRole('button', { name: 'Развернуть меню' })).toBeTruthy()
    first.unmount()
    await renderAt('/')
    expect(sidebar().getByRole('button', { name: 'Развернуть меню' })).toBeTruthy()
  })

  it('shows the pending count on Платежи, across all dates', async () => {
    const fetchMock = stubFetch(3)
    await renderAt('/')
    expect(sidebar().getByRole('button', { name: /Платежи/ }).textContent).toBe('Платежи3')
    const url = fetchMock.mock.calls.map(([u]) => u).find((u) => u.startsWith('/api/payments?status=pending'))!
    expect(new URLSearchParams(url.split('?')[1]).get('from')).toBeTruthy()
  })

  it('survives a failed pending count without a badge', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(500, {})))
    await renderAt('/')
    expect(sidebar().getByRole('button', { name: /Платежи/ }).textContent).toBe('Платежи')
  })

  it('header search opens users with the query', async () => {
    const fetchMock = stubFetch()
    await renderAt('/')
    const field = screen.getByRole('searchbox', { name: 'Поиск пользователей' })
    fireEvent.change(field, { target: { value: ' @ann ' } })
    fireEvent.submit(field.closest('form')!)
    await flush()
    expect(activeLabel()).toBe('Пользователи')
    expect(fetchMock.mock.calls.map(([u]) => u)).toContain('/api/users?q=%40ann')
  })

  it('takes the owner and the bot from /api/me, nothing is hard-coded', async () => {
    const fetchMock = stubFetch(0, { telegramId: 1, username: 'owner_nick', name: 'Owner Name', bot: { username: 'shop_bot', name: 'Shop Bot' } })
    await renderAt('/')
    expect(fetchMock.mock.calls.filter(([u]) => u === '/api/me')).toHaveLength(1)
    expect(within(document.querySelector('header')!).getByText('@owner_nick')).toBeTruthy()
    expect(screen.getByText('Shop Bot')).toBeTruthy()
    expect(screen.getByText('@shop_bot')).toBeTruthy()
    expect(document.querySelector('img[src="/api/avatars/bot"]')).not.toBeNull()
    expect(document.querySelector('header img')!.getAttribute('src')).toBe('/api/avatars/1')
    // навигация не перечитывает /api/me
    fireEvent.click(sidebar().getByRole('button', { name: /Платежи/ }))
    await flush()
    expect(fetchMock.mock.calls.filter(([u]) => u === '/api/me')).toHaveLength(1)
  })

  it('without /api/me data hides the owner and the bot instead of showing old names', async () => {
    stubFetch()
    await renderAt('/')
    expect(within(document.querySelector('header')!).queryByText('Владелец')).toBeNull()
    expect(document.body.textContent).not.toMatch(/bruhabruh4|bruhanet_payment_bot|BruhaNet Payment|Bruha Bruh/)
  })

  it('an empty bot in /api/me (Telegram down) is the same as no data', async () => {
    stubFetch(0, { telegramId: 1, username: '', name: '', bot: { username: '', name: '' } })
    await renderAt('/')
    expect(within(document.querySelector('header')!).queryByText('Владелец')).toBeNull()
    expect(screen.queryByText(/^@/)).toBeNull()
  })

  it('toggles the theme from the header', async () => {
    stubFetch()
    await renderAt('/')
    fireEvent.click(screen.getByRole('button', { name: 'Тёмная тема' }))
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(screen.getByRole('button', { name: 'Светлая тема' })).toBeTruthy()
  })

  describe('sync with the panel', () => {
    const syncButton = () => screen.getByRole('button', { name: 'Синхронизировать с панелью' })
    const posts = (f: ReturnType<typeof vi.fn>) => f.mock.calls.filter(([u, o]) => u === '/api/sync' && (o as RequestInit).method === 'POST')

    function stubSync(sync: () => Response | Promise<Response>) {
      const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
        if (url === '/api/sync') return init?.method === 'POST' ? sync() : json(200, { lastSyncAt: '2026-10-05T10:00:00Z' })
        if (url.startsWith('/api/payments')) return json(200, { items: [], total: 0, revenue: 0 })
        return json(200, { items: [], total: 0, page: 1, pages: 1 })
      })
      vi.stubGlobal('fetch', fetchMock)
      return fetchMock
    }

    it('replaces the bell and shows the last sync in the title', async () => {
      stubSync(() => json(200, {}))
      await renderAt('/')
      expect(screen.queryByRole('button', { name: 'Уведомления' })).toBeNull()
      expect(syncButton().getAttribute('title')).toMatch(/^Синхронизировать с панелью\. Последняя: 5 окт.*13:00$/)
    })

    it('syncs once on a double click, toasts the count and reloads the screen', async () => {
      const f = stubSync(() => json(200, { total: 1200, updated: 3, finishedAt: '2026-11-01T10:00:00Z' }))
      await renderAt('/users')
      const before = f.mock.calls.filter(([u]) => u.startsWith('/api/users?')).length
      fireEvent.click(syncButton())
      fireEvent.click(syncButton())
      expect((syncButton() as HTMLButtonElement).disabled).toBe(true)
      await flush()
      expect(posts(f)).toHaveLength(1)
      expect(screen.getByText(/^Синхронизировано: 1\s200$/)).toBeTruthy()
      expect((syncButton() as HTMLButtonElement).disabled).toBe(false)
      expect(syncButton().getAttribute('title')).toContain('1 нояб')
      expect(f.mock.calls.filter(([u]) => u.startsWith('/api/users?')).length).toBe(before + 1)
    })

    it('toasts the server message on failure and does not reload', async () => {
      const f = stubSync(() => json(409, { error: 'sync_running', message: 'Синхронизация уже идёт' }))
      await renderAt('/users')
      const before = f.mock.calls.filter(([u]) => u.startsWith('/api/users?')).length
      fireEvent.click(syncButton())
      await flush()
      expect(screen.getByText('Синхронизация уже идёт')).toBeTruthy()
      expect(f.mock.calls.filter(([u]) => u.startsWith('/api/users?')).length).toBe(before)
      expect((syncButton() as HTMLButtonElement).disabled).toBe(false)
    })
  })

  it('logs out through the sidebar', async () => {
    const fetchMock = stubFetch()
    await renderAt('/')
    fireEvent.click(sidebar().getByRole('button', { name: 'Выйти' }))
    await flush()
    expect(fetchMock.mock.calls.map(([u]) => u)).toContain('/api/logout')
    expect(screen.getByRole('heading', { level: 1, name: 'Войдите через Telegram' })).toBeTruthy()
  })

  it('sends any 401 to /login-required', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 401 })))
    await renderAt('/payments')
    await act(async () => {
      await expect(get('/api/payments')).rejects.toThrow('unauthorized')
    })
    expect(screen.getByRole('heading', { level: 1, name: 'Войдите через Telegram' })).toBeTruthy()
    expect(document.querySelector('aside nav')).toBeNull()
    expect(document.title).toBe('Вход в админ-панель — BruhaNet')
  })
})
