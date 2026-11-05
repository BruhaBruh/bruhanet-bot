import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { get } from '../api'
import { App } from '../App'

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })

function stubFetch(pending = 0) {
  const fetchMock = vi.fn(async (url: string) =>
    url.startsWith('/api/payments') ? json(200, { items: [], total: pending, revenue: 0 }) : url.startsWith('/api/users?') || url === '/api/logout' ? json(200, { items: [] }) : json(500, {}),
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
      'ОбзорМаркетингРассылкиПромокодыБотРедактор сообщенийДанныеПользователиПлатежиСистемаНастройки',
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
    const field = screen.getByRole('searchbox', { name: 'Поиск по пользователям и платежам' })
    fireEvent.change(field, { target: { value: ' @ann ' } })
    fireEvent.submit(field.closest('form')!)
    await flush()
    expect(activeLabel()).toBe('Пользователи')
    expect(fetchMock.mock.calls.map(([u]) => u)).toContain('/api/users?q=%40ann')
  })

  it('toggles the theme from the header', async () => {
    stubFetch()
    await renderAt('/')
    fireEvent.click(screen.getByRole('button', { name: 'Тёмная тема' }))
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(screen.getByRole('button', { name: 'Светлая тема' })).toBeTruthy()
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
