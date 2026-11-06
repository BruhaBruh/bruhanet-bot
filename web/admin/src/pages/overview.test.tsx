import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
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
  overview?: Reply
  upcoming?: Reply
  recent?: Reply
}

const overview = (over: object = {}) =>
  json(200, {
    users: { total: 3214, new7: 12, prev7: 9 },
    promos: { active: 2, activations7: 5 },
    pending: 4,
    revenue: { week: 84300, prevWeek: 75268 },
    ...over,
  })

function stub(r: Replies = {}) {
  const fetchMock = vi.fn(async (url: string) => {
    if (url === '/api/overview') return r.overview ? r.overview() : overview()
    if (url === '/api/broadcasts?status=scheduled') return r.upcoming ? r.upcoming() : json(200, { items: [], total: 0 })
    if (url.startsWith('/api/payments?from=2000-')) return r.recent ? r.recent() : json(200, { items: [], total: 0, revenue: 0 })
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
    const labels = ['Пользователи', 'Выручка за 7 дней', 'Платежи на проверке', 'Активные промокоды']
    const at = labels.map((l) => document.body.textContent!.indexOf(l))
    expect(at).toEqual([...at].sort((a, b) => a - b))
    expect(card('Пользователи').textContent).toContain('3 214')
    expect(card('Выручка за 7 дней').textContent).toContain('84 300 ₽')
    expect(card('Активные промокоды').textContent).toContain('2')
    expect(card('Платежи на проверке').textContent).toContain('4')
    expect(screen.queryByText('Открываемость рассылок')).toBeNull()
  })

  it('computes the revenue delta against the previous 7 days', async () => {
    stub()
    await renderPage()
    expect(within(card('Выручка за 7 дней')).getByText('+12%')).toBeTruthy()
    expect(within(card('Выручка за 7 дней')).getByText('к прошлой неделе')).toBeTruthy()
  })

  it('hides the delta on a zero base', async () => {
    stub({ overview: () => overview({ revenue: { week: 84300, prevWeek: 0 } }) })
    await renderPage()
    expect(card('Выручка за 7 дней').textContent).toMatch(/84\s300\s₽/)
    expect(screen.queryByText(/%$/)).toBeNull()
    expect(screen.queryByText('к прошлой неделе')).toBeNull()
  })

  it('users show the real +N for the last 7 days, promos the activations of the week', async () => {
    stub()
    await renderPage()
    expect(within(card('Пользователи')).getByText('+12')).toBeTruthy()
    expect(within(card('Пользователи')).getByText('за 7 дней')).toBeTruthy()
    expect(within(card('Активные промокоды')).getByText('5')).toBeTruthy()
    expect(within(card('Активные промокоды')).getByText('применений за неделю')).toBeTruthy()
    // данные есть — прочерков в карточках нет
    expect(screen.queryAllByText('—')).toHaveLength(0)
  })

  it('zero new users is a neutral +0, not a dash', async () => {
    stub({ overview: () => overview({ users: { total: 3, new7: 0, prev7: 0 }, promos: { active: 0, activations7: 0 } }) })
    await renderPage()
    expect(within(card('Пользователи')).getByText('+0')).toBeTruthy()
    expect(within(card('Активные промокоды')).getByText('применений за неделю')).toBeTruthy()
  })

  it('a section the server could not count is a dash; the other cards stay', async () => {
    stub({ overview: () => overview({ users: null }) })
    await renderPage()
    expect(within(card('Пользователи')).getByText('—')).toBeTruthy()
    expect(screen.queryByText('за 7 дней')).toBeNull()
    expect(card('Выручка за 7 дней').textContent).toMatch(/84\s300\s₽/)
    expect(card('Активные промокоды').textContent).toContain('2')
  })

  it('the whole aggregate failing leaves payments and broadcasts alone', async () => {
    stub({ overview: () => json(500, {}), upcoming: () => json(200, { items: [{ id: 'a', status: 'scheduled', text: 'Акция', scheduledAt: '2026-11-07T15:30:00Z' }], total: 1 }) })
    await renderPage()
    for (const l of ['Пользователи', 'Выручка за 7 дней', 'Платежи на проверке', 'Активные промокоды']) {
      expect(within(card(l)).getByText('—')).toBeTruthy()
    }
    expect(screen.getByRole('link', { name: /Акция/ })).toBeTruthy()
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
  it('no scheduled broadcasts: empty state', async () => {
    stub()
    await renderPage()
    expect(screen.getByRole('heading', { name: 'Ближайшие рассылки' })).toBeTruthy()
    expect(screen.getByText('Запланированных рассылок нет')).toBeTruthy()
  })

  it('shows up to three scheduled broadcasts with their Moscow time, each links to its screen', async () => {
    const b = (id: string, at: string) => ({ id, status: 'scheduled', text: `<b>Акция ${id}</b>`, scheduledAt: at })
    stub({
      upcoming: () =>
        json(200, {
          items: [b('a', '2026-11-07T15:30:00Z'), b('b', '2026-11-08T07:00:00Z'), b('c', '2026-11-09T09:15:00Z'), b('d', '2026-11-10T09:15:00Z')],
          total: 4,
        }),
    })
    await renderPage()
    const link = (name: RegExp) => screen.getByRole('link', { name })
    expect(link(/Акция a/).getAttribute('href')).toBe('/broadcasts/a')
    expect(link(/Акция a/).textContent).toContain('7 нояб. 2026 в 18:30 (МСК)')
    expect(link(/Акция b/).textContent).toContain('8 нояб. 2026 в 10:00 (МСК)')
    expect(link(/Акция c/).getAttribute('href')).toBe('/broadcasts/c')
    expect(screen.queryByRole('link', { name: /Акция d/ })).toBeNull()
    expect(screen.queryByText('Запланированных рассылок нет')).toBeNull()
  })

  it('a failed load of scheduled broadcasts is said so', async () => {
    stub({ upcoming: () => json(500, {}) })
    await renderPage()
    expect(screen.getByText('Не удалось загрузить рассылки')).toBeTruthy()
  })

  it('«Платежи на проверке» links to the pending tab', async () => {
    stub({ overview: () => overview({ pending: 7 }) })
    await renderPage()
    const link = screen.getByRole('link', { name: /Платежи на проверке/ })
    expect(link.getAttribute('href')).toBe('/payments?tab=pending')
    expect(link.textContent).toContain('7')
    expect(link.textContent).toContain('ждут решения')
  })

  it('an empty queue and a failed count', async () => {
    stub({ overview: () => overview({ pending: 0 }) })
    await renderPage()
    expect(card('Платежи на проверке').textContent).toContain('очередь пуста')
  })

  it('a failed pending count shows a dash', async () => {
    stub({ overview: () => overview({ pending: null }) })
    await renderPage()
    expect(within(card('Платежи на проверке')).getByText('—')).toBeTruthy()
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
