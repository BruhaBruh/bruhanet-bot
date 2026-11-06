import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'

afterEach(() => vi.unstubAllGlobals())

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })
const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

const payment = (id: number, status = 'confirmed', over = {}) => ({
  id,
  telegramId: 5,
  username: 'eve',
  month: 3,
  price: 900,
  status,
  createdAt: '2026-11-01T10:00:00Z',
  ...over,
})

type Reply = (params: URLSearchParams) => Response | Promise<Response>
const page: Reply = () => json(200, { items: [payment(1)], total: 120, revenue: 900 })

// Запросы экрана отдельно от бейджа в шапке (status=pending&from=2000-01-01).
function stub(reply: Reply = page) {
  const calls: URLSearchParams[] = []
  vi.stubGlobal('fetch', async (url: string) => {
    if (!url.startsWith('/api/payments')) return new Promise(() => {})
    const params = new URLSearchParams(url.split('?')[1])
    if (params.get('from')?.startsWith('2000-')) return json(200, { items: [], total: 0, revenue: 0 })
    calls.push(params)
    return reply(params)
  })
  return calls
}

const renderPage = async () => {
  render(
    <MemoryRouter initialEntries={['/payments']}>
      <App />
    </MemoryRouter>,
  )
  await flush()
}

const table = () => within(screen.getByRole('table'))
const pick = async (name: string) => {
  fireEvent.click(screen.getByRole('radio', { name }))
  await flush()
}
const next = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
  await flush()
}

describe('Payments', () => {
  it('shows macro columns, revenue and the pager', async () => {
    stub()
    await renderPage()
    expect(table().getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['ID', 'Пользователь', 'Тариф', 'Способ', 'Сумма', 'Статус', 'Дата', 'Действия'])
    const cells = table().getAllByRole('cell').map((c) => c.textContent)
    expect(cells[0]).toBe('1')
    expect(cells[2]).toBe('3 мес.')
    expect(cells[3]).toBe('—')
    expect(cells[4]).toBe('900 ₽')
    expect(cells[5]).toBe('Оплачено')
    expect(screen.getByText('Выручка по дате подтверждения за период')).toBeTruthy()
    expect(screen.getAllByText(/^900\s₽$/).length).toBeGreaterThan(1)
    expect(screen.getByText(/^Показано 1–1 из 120$/)).toBeTruthy()
    expect(screen.getByText('1 из 3')).toBeTruthy()
  })

  it('links the user to the card and opens it on row click', async () => {
    stub()
    await renderPage()
    expect(table().getByText('@eve').closest('a')?.getAttribute('href')).toBe('/users/5')
    fireEvent.click(table().getByText(/^3\sмес\.$/))
    await flush()
    expect(screen.getByRole('button', { name: 'Все пользователи' })).toBeTruthy()
  })

  it('copies the id with a toast without opening the card', async () => {
    stub()
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => {}) } })
    await renderPage()
    fireEvent.click(table().getByRole('button', { name: 'Скопировать ID платежа' }))
    await flush()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('1')
    expect(screen.getByText('ID 1 скопирован')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Все пользователи' })).toBeNull()
  })

  it('maps segments to API statuses', async () => {
    const calls = stub()
    await renderPage()
    expect(calls.at(-1)?.get('status')).toBe('')
    await pick('Оплачено')
    expect(calls.at(-1)?.get('status')).toBe('confirmed')
    await pick('Ожидают')
    expect(calls.at(-1)?.get('status')).toBe('pending')
  })

  it.each([
    ['confirmed', 'Оплачено'],
    ['pending', 'Ожидает'],
    ['declined', 'Отклонён'],
    ['cancelled', 'Отменён'],
  ])('tags status %s as %s', async (status, label) => {
    stub(() => json(200, { items: [payment(1, status)], total: 1, revenue: 0 }))
    await renderPage()
    expect(table().getByText(label)).toBeTruthy()
  })

  it('shows refunds as an empty state, revenue stays', async () => {
    stub()
    await renderPage()
    await pick('Возвраты')
    expect(table().queryByText('@eve')).toBeNull()
    expect(table().getByText('Возвратов нет')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Следующая страница' })).toBeNull()
    expect(screen.getByText(/^900\s₽$/)).toBeTruthy()
  })

  it('pages through the API', async () => {
    const calls = stub()
    await renderPage()
    await next()
    expect(calls.at(-1)?.get('page')).toBe('2')
    expect(screen.getByText(/^Показано 51–51 из 120$/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Предыдущая страница' }))
    await flush()
    expect(calls.at(-1)?.get('page')).toBe('1')
  })

  it('resets to page 1 when the segment changes', async () => {
    const calls = stub()
    await renderPage()
    await next()
    await pick('Оплачено')
    expect(calls.at(-1)?.get('page')).toBe('1')
    expect(calls.at(-1)?.get('status')).toBe('confirmed')
  })

  it('opens the period under «Фильтры» and a new date resets to page 1', async () => {
    const calls = stub()
    await renderPage()
    expect(screen.queryByLabelText('С даты')).toBeNull()
    await next()
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    fireEvent.change(screen.getByLabelText('С даты'), { target: { value: '2026-01-15' } })
    await flush()
    expect(calls.at(-1)?.get('from')).toBe('2026-01-15')
    expect(calls.at(-1)?.get('page')).toBe('1')
    expect(screen.getByText(/^с 15 янв\./)).toBeTruthy()
  })

  it('a status from the filters overrides the tab, a tab click resets it', async () => {
    const calls = stub()
    await renderPage()
    await pick('Возвраты')
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    expect(screen.getByRole('radio', { name: 'Любой' }).getAttribute('aria-checked')).toBe('true')
    await pick('Отменён')
    expect(calls.at(-1)?.get('status')).toBe('cancelled')
    expect(table().getByText('@eve')).toBeTruthy()
    expect(screen.getByRole('radio', { name: 'Возвраты' }).getAttribute('aria-checked')).toBe('false')
    await pick('Отклонён')
    expect(calls.at(-1)?.get('status')).toBe('declined')
    await pick('Ожидают')
    expect(calls.at(-1)?.get('status')).toBe('pending')
    expect(screen.getByRole('radio', { name: 'Ожидают' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('radio', { name: 'Любой' }).getAttribute('aria-checked')).toBe('true')
  })

  it('both dates cleared: omits them and says the server uses 30 days', async () => {
    const calls = stub()
    await renderPage()
    expect(screen.queryByText('Даты не выбраны: последние 30 дней')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    fireEvent.change(screen.getByLabelText('С даты'), { target: { value: '' } })
    fireEvent.change(screen.getByLabelText('По дату'), { target: { value: '' } })
    await flush()
    expect(calls.at(-1)?.has('from')).toBe(false)
    expect(calls.at(-1)?.has('to')).toBe(false)
    expect(screen.getByText('Даты не выбраны: последние 30 дней')).toBeTruthy()
  })

  it('sends the default 30-day period', async () => {
    const calls = stub()
    await renderPage()
    const days = (Date.parse(calls[0].get('to')!) - Date.parse(calls[0].get('from')!)) / 864e5
    expect(days).toBe(29)
  })

  it('ignores a stale response', async () => {
    let releaseAll!: () => void
    stub((params) =>
      params.get('status') === ''
        ? new Promise((r) => (releaseAll = () => r(json(200, { items: [payment(1, 'pending', { username: 'old' })], total: 1, revenue: 1 }))))
        : json(200, { items: [payment(2, 'confirmed', { username: 'new' })], total: 1, revenue: 2 }),
    )
    await renderPage()
    await pick('Оплачено')
    await act(async () => releaseAll())
    await flush()
    expect(table().getByText('@new')).toBeTruthy()
    expect(table().queryByText('@old')).toBeNull()
  })

  it('reports a load error and hides stale numbers', async () => {
    let fail = false
    stub(() => (fail ? json(500, {}) : page(new URLSearchParams())))
    await renderPage()
    fail = true
    await pick('Оплачено')
    expect(table().getByText('Не удалось загрузить платежи')).toBeTruthy()
    expect(table().queryByText('@eve')).toBeNull()
    expect(screen.queryByText(/^900\s₽$/)).toBeNull()
  })
})
