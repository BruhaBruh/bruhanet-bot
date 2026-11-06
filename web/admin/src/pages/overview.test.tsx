import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mskDate } from '../format'
import Overview from './Overview'

afterEach(() => vi.unstubAllGlobals())

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })
const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

const payment = (id: number, status: string, over = {}) => ({
  id,
  telegramId: 100 + id,
  username: `u${id}`,
  month: 1,
  price: 11900,
  status,
  createdAt: new Date().toISOString(),
  ...over,
})

type Reply = () => Response
interface Replies {
  users?: Reply
  promos?: Reply
  week?: Reply
  prevWeek?: Reply
  recent?: Reply
}

function stub(r: Replies = {}) {
  const now = new Date()
  const page = (revenue: number, items: unknown[] = []) => json(200, { items, total: items.length, revenue })
  const fetchMock = vi.fn(async (url: string) => {
    if (url === '/api/users') return r.users ? r.users() : json(200, { items: [], total: 3214 })
    if (url === '/api/promos?status=active') return r.promos ? r.promos() : json(200, { items: [{ id: 1 }, { id: 2 }] })
    if (url === `/api/payments?from=${mskDate(now, -6)}&to=${mskDate(now)}`) return r.week ? r.week() : page(84300)
    if (url === `/api/payments?from=${mskDate(now, -13)}&to=${mskDate(now, -7)}`) return r.prevWeek ? r.prevWeek() : page(75268)
    if (url.startsWith('/api/payments?from=2000-')) return r.recent ? r.recent() : page(0)
    return json(500, {})
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function renderPage() {
  render(
    <MemoryRouter>
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/payments" element={<p>страница платежей</p>} />
      </Routes>
    </MemoryRouter>,
  )
  await flush()
}

// Карточка — ближайший предок подписи, в котором есть значение.
const card = (label: string) => screen.getByText(label).parentElement!.parentElement!

describe('Overview cards', () => {
  it('shows the four cards in macro order with real numbers', async () => {
    stub()
    await renderPage()
    const labels = ['Пользователи', 'Выручка за 7 дней', 'Открываемость рассылок', 'Активные промокоды']
    const at = labels.map((l) => document.body.textContent!.indexOf(l))
    expect(at).toEqual([...at].sort((a, b) => a - b))
    expect(card('Пользователи').textContent).toContain('3 214')
    expect(card('Выручка за 7 дней').textContent).toContain('84 300 ₽')
    expect(card('Активные промокоды').textContent).toContain('2')
    // значение и дельта, как у соседних карточек
    expect(within(card('Открываемость рассылок')).getAllByText('—')).toHaveLength(2)
  })

  it('computes the revenue delta against the previous 7 days', async () => {
    stub()
    await renderPage()
    expect(within(card('Выручка за 7 дней')).getByText('+12%')).toBeTruthy()
    expect(within(card('Выручка за 7 дней')).getByText('к прошлой неделе')).toBeTruthy()
  })

  it('hides the delta on a zero base', async () => {
    stub({ prevWeek: () => json(200, { items: [], total: 0, revenue: 0 }) })
    await renderPage()
    expect(card('Выручка за 7 дней').textContent).toContain('84 300 ₽')
    expect(screen.queryByText(/%$/)).toBeNull()
    expect(screen.queryByText('к прошлой неделе')).toBeNull()
  })

  it('hides the delta when the previous period failed', async () => {
    stub({ prevWeek: () => json(500, {}) })
    await renderPage()
    expect(card('Выручка за 7 дней').textContent).toContain('84 300 ₽')
    expect(screen.queryByText('к прошлой неделе')).toBeNull()
  })

  it('users have no backend delta: drawn as a dash', async () => {
    stub()
    await renderPage()
    expect(within(card('Пользователи')).getByText('—')).toBeTruthy()
    expect(within(card('Пользователи')).getByText('за 7 дней')).toBeTruthy()
  })

  it('one failure leaves the other cards alone', async () => {
    stub({ users: () => json(500, {}) })
    await renderPage()
    expect(within(card('Пользователи')).getByText('—')).toBeTruthy()
    expect(screen.queryByText('за 7 дней')).toBeNull()
    expect(card('Выручка за 7 дней').textContent).toContain('84 300 ₽')
    expect(card('Активные промокоды').textContent).toContain('2')
  })

  it('every section failing still renders the page', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(500, {})))
    await renderPage()
    expect(within(card('Выручка за 7 дней')).getByText('—')).toBeTruthy()
    expect(within(card('Активные промокоды')).getByText('—')).toBeTruthy()
    expect(screen.getAllByText('Не удалось загрузить платежи').length).toBeGreaterThan(0)
    expect(screen.getByText('Новая рассылка')).toBeTruthy()
  })
})

describe('Overview payments', () => {
  it('shows the last five with status tags and short columns', async () => {
    const items = [
      payment(9, 'confirmed'),
      payment(8, 'pending'),
      payment(7, 'declined'),
      payment(6, 'cancelled', { username: '' }),
      payment(5, 'confirmed'),
      payment(4, 'confirmed'),
    ]
    stub({ recent: () => json(200, { items, total: 6, revenue: 0 }) })
    await renderPage()
    const table = within(screen.getByRole('table'))
    expect(table.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['ID', 'Пользователь', 'Сумма', 'Статус', 'Дата'])
    expect(table.getAllByRole('row')).toHaveLength(6)
    for (const label of ['Оплачено', 'Ожидает', 'Отклонён', 'Отменён']) expect(table.getAllByText(label).length).toBeGreaterThan(0)
    expect(table.getByText('@u9')).toBeTruthy()
    expect(table.getByText('ID 106')).toBeTruthy() // без username — telegram id
    expect(table.getAllByText(/^11\s900\s₽$/)).toHaveLength(5)
    expect(table.getAllByText(/^сегодня, /)).toHaveLength(5)
    expect(table.queryByText('@u4')).toBeNull()
  })

  it('asks for payments past the default 30 days', async () => {
    const fetchMock = stub()
    await renderPage()
    expect(fetchMock.mock.calls.some(([u]) => u.startsWith('/api/payments?from=2000-'))).toBe(true)
  })

  it('says so when there are none', async () => {
    stub()
    await renderPage()
    expect(within(screen.getByRole('table')).getByText('Платежей пока нет')).toBeTruthy()
  })

  it('renders a mobile row for each payment', async () => {
    stub({ recent: () => json(200, { items: [payment(9, 'pending')], total: 1, revenue: 0 }) })
    await renderPage()
    expect(screen.getAllByText('@u9')).toHaveLength(2) // таблица и Cell, переключает CSS
  })

  it('«Все платежи» goes to the payments screen', async () => {
    stub()
    await renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Все платежи' }))
    expect(screen.getByText('страница платежей')).toBeTruthy()
  })
})

describe('Overview side column', () => {
  it('upcoming broadcasts are an empty state', async () => {
    stub()
    await renderPage()
    expect(screen.getByRole('heading', { name: 'Ближайшие рассылки' })).toBeTruthy()
    expect(screen.getByText('Запланированных рассылок нет')).toBeTruthy()
  })

  it('quick actions link to their screens', async () => {
    stub()
    await renderPage()
    const href = (name: string) => screen.getByRole('link', { name }).getAttribute('href')
    expect(href('Новая рассылка')).toBe('/broadcasts/new')
    expect(href('Создать промокод')).toBe('/promos/new')
    expect(href('Редактор сообщений')).toBe('/texts')
  })
})
