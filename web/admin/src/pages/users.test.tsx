import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { useEffect } from 'react'
import { MemoryRouter, useLocation, useNavigate, type NavigateFunction } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'

vi.mock('../compose/Composer', () => ({
  Composer: ({ onSent }: { onSent: (m: unknown) => void }) => (
    <button onClick={() => onSent({ id: 9, telegramId: 7, text: 'привет', status: 'pending', createdAt: '2026-11-01T10:00:00Z' })}>Послать</button>
  ),
}))

afterEach(() => vi.unstubAllGlobals())

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })
const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

const user = (id: number, username: string, confirmed = 0, over = {}) => ({
  telegramId: id,
  username,
  name: '',
  createdAt: '2026-09-12T10:00:00Z',
  confirmed,
  confirmedSum: confirmed * 300,
  expireAt: null,
  panelStatus: null,
  blockedBotAt: null,
  tariffMonths: null,
  lastSeenAt: null,
  broadcastsReceived: 0,
  ...over,
})
const page = (items: unknown[], over = {}) => ({ items, total: items.length, page: 1, pages: 1, ...over })

const detail = (over = {}) => ({
  user: user(7, 'bob', 2),
  stats: { confirmed: 2, confirmedSum: 600, unpaidAdvances: 1, lastConfirmedAt: '2026-10-05T10:00:00Z', firstConfirmedAt: '2026-09-15T10:00:00Z', hasHistory: true },
  payments: [{ id: 31, telegramId: 7, username: 'bob', month: 3, price: 900, status: 'confirmed', createdAt: '2026-11-01T10:00:00Z' }],
  panel: { id: 1234, expireAt: '2027-01-20T10:00:00Z', status: 'ACTIVE', error: null },
  messages: [],
  promos: [],
  ...over,
})

// ответы по префиксу пути; бейдж платежей в шапке отвечает сам
function stub(routes: Record<string, () => Response | Promise<Response>>) {
  const fetchMock = vi.fn(async (url: string) => {
    if (url.startsWith('/api/payments')) return json(200, { items: [], total: 0, revenue: 0 })
    const k = Object.keys(routes).find((p) => url.startsWith(p))
    return k ? routes[k]() : json(500, {})
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const router = { go: (() => {}) as NavigateFunction, search: '' }
const Grab = () => {
  const go = useNavigate()
  const { search } = useLocation()
  useEffect(() => {
    router.go = go
    router.search = search
  })
  return null
}

const renderAt = async (path: string) => {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Grab />
      <App />
    </MemoryRouter>,
  )
  await flush()
}

const searchBox = () => screen.getByRole<HTMLInputElement>('searchbox', { name: 'Поиск пользователей' })
const userCalls = (f: ReturnType<typeof stub>) => f.mock.calls.map(([u]) => u).filter((u) => u.startsWith('/api/users'))

describe('Users', () => {
  it('shows macro columns with tariff and status', async () => {
    stub({ '/api/users?': () => json(200, page([user(1, 'ann', 2, { tariffMonths: 3, panelStatus: 'ACTIVE' }), user(2, '')])) })
    await renderAt('/users')
    const table = within(screen.getByRole('table'))
    expect(table.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Пользователь', 'Telegram ID', 'Тариф', 'Платежей', 'Оплачено всего', 'С нами с', 'Статус', 'Действия'])
    const cells = within(table.getByText('@ann').closest('tr')!).getAllByRole('cell').map((c) => c.textContent)
    expect(cells.slice(1, 7)).toEqual(['1', '3 мес.', '2', '600 ₽', '12 сент. 2026', 'активна'])
    const second = within(table.getByText('ID 2').closest('tr')!).getAllByRole('cell').map((c) => c.textContent)
    expect([second[2], second[6]]).toEqual(['—', '—'])
  })

  it('opens the card on row click and from more', async () => {
    stub({ '/api/users?': () => json(200, page([user(1, 'ann')])), '/api/users/1': () => new Promise(() => {}) })
    await renderAt('/users')
    fireEvent.click(within(screen.getByRole('table')).getByRole('button', { name: 'Действия с пользователем' }))
    await flush()
    expect(screen.getByRole('button', { name: 'Все пользователи' })).toBeTruthy()
  })

  it('picks up ?q= into the field and the request', async () => {
    const f = stub({ '/api/users?': () => json(200, page([])) })
    await renderAt('/users?q=ann')
    expect(searchBox().value).toBe('ann')
    expect(userCalls(f)).toEqual(['/api/users?q=ann'])
    expect(within(screen.getByRole('table')).getByText('Ничего не найдено')).toBeTruthy()
  })

  it('follows ?q= changed from outside, like the header search', async () => {
    const f = stub({ '/api/users?': () => json(200, page([])) })
    await renderAt('/users?q=ann')
    await act(async () => router.go('/users?q=bob'))
    await flush()
    expect(searchBox().value).toBe('bob')
    expect(userCalls(f).at(-1)).toBe('/api/users?q=bob')
  })

  it('debounces typing into ?q= and one request', async () => {
    const f = stub({ '/api/users?': () => json(200, page([])) })
    await renderAt('/users')
    vi.useFakeTimers()
    try {
      fireEvent.change(searchBox(), { target: { value: 'a' } })
      await act(() => vi.advanceTimersByTimeAsync(200))
      fireEvent.change(searchBox(), { target: { value: 'an' } })
      await act(() => vi.advanceTimersByTimeAsync(299))
      expect(router.search).toBe('')
      await act(() => vi.advanceTimersByTimeAsync(1))
      expect(router.search).toBe('?q=an')
      expect(searchBox().value).toBe('an')
      expect(userCalls(f)).toEqual(['/api/users?q=', '/api/users?q=an'])
    } finally {
      vi.useRealTimers()
    }
  })

  it('ignores a stale response', async () => {
    let releaseOld!: () => void
    stub({
      '/api/users?q=old': () => new Promise<Response>((r) => (releaseOld = () => r(json(200, page([user(1, 'old')]))))),
      '/api/users?q=new': () => json(200, page([user(2, 'new')])),
    })
    await renderAt('/users?q=old')
    await act(async () => router.go('/users?q=new'))
    await flush()
    await act(async () => releaseOld())
    await flush()
    const table = within(screen.getByRole('table'))
    expect(table.getByText('@new')).toBeTruthy()
    expect(table.queryByText('@old')).toBeNull()
  })

  it('sends the tab to the API and shows its empty state', async () => {
    const f = stub({ '/api/users?': () => json(200, page([])) })
    await renderAt('/users')
    fireEvent.click(screen.getByRole('radio', { name: 'Заблокировали' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&tab=blocked_bot')
    expect(within(screen.getByRole('table')).getByText('Заблокировавших бота нет')).toBeTruthy()
    fireEvent.click(screen.getByRole('radio', { name: 'Активные' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&tab=active')
    fireEvent.click(screen.getByRole('radio', { name: 'Пробный период' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&tab=trial')
  })

  it('shows name with username under it and «заблокировал бота» over the panel status', async () => {
    stub({ '/api/users?': () => json(200, page([user(1, 'ann', 0, { name: 'Анна К', panelStatus: 'ACTIVE', blockedBotAt: '2026-11-01T10:00:00Z' })])) })
    await renderAt('/users')
    const table = within(screen.getByRole('table'))
    expect(table.getByText('Анна К')).toBeTruthy()
    expect(table.getByText('@ann')).toBeTruthy()
    expect(table.getByText('заблокировал бота')).toBeTruthy()
    expect(table.queryByText('активна')).toBeNull()
  })

  it('paginates with the real total and resets the page on a filter', async () => {
    const f = stub({ '/api/users?': () => json(200, page([user(1, 'ann')], { total: 51, pages: 2 })) })
    await renderAt('/users')
    expect(screen.getAllByText(/из 51/).length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&page=2')
    fireEvent.click(screen.getByRole('radio', { name: 'Активные' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&tab=active')
  })

  it('«Фильтры» opens the card and filters on the server', async () => {
    const f = stub({ '/api/users?': () => json(200, page([])) })
    await renderAt('/users')
    expect(screen.queryByText('Есть оплаты')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Есть оплаты' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&hasPayments=true')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Есть неоплаченные авансы' }))
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&hasPayments=true&hasUnpaidAdvances=true')
    fireEvent.change(screen.getByLabelText('Регистрация с'), { target: { value: '2026-10-01' } })
    await flush()
    fireEvent.change(screen.getByLabelText('Регистрация по'), { target: { value: '2026-10-31' } })
    await flush()
    expect(userCalls(f).at(-1)).toBe('/api/users?q=&hasPayments=true&hasUnpaidAdvances=true&from=2026-10-01&to=2026-10-31')
  })
})

describe('UserCard', () => {
  it('says the user is not found on 404', async () => {
    stub({ '/api/users/7': () => json(404, { error: 'not_found' }) })
    await renderAt('/users/7')
    expect(screen.getByRole('alert').textContent).toBe('Пользователь не найден')
    expect(screen.getByRole('button', { name: 'Все пользователи' })).toBeTruthy()
  })

  it('resets state when id changes', async () => {
    // 7 — 404, 8 — ещё грузится: ошибка от 7 не должна остаться на карточке 8
    stub({ '/api/users/7': () => json(404, { error: 'not_found' }), '/api/users/8': () => new Promise(() => {}) })
    await renderAt('/users/7')
    expect(screen.getByRole('alert')).toBeTruthy()
    await act(async () => router.go('/users/8'))
    await flush()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByRole('status', { name: 'Загрузка' })).toBeTruthy()
  })

  it('names the user in the header and restores the section title', async () => {
    stub({ '/api/users/7': () => json(200, detail()), '/api/users?': () => json(200, page([])) })
    await renderAt('/users/7')
    expect(screen.getByRole('heading', { level: 1, name: '@bob' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Все пользователи' }))
    await flush()
    expect(screen.getByRole('heading', { level: 1, name: 'Пользователи' })).toBeTruthy()
  })

  it('shows head, facts and payments from real data', async () => {
    stub({ '/api/users/7': () => json(200, detail()) })
    await renderAt('/users/7')
    expect(screen.getByRole('link', { name: '@bob' }).getAttribute('href')).toBe('https://t.me/bob')
    expect(screen.getByText('с нами с 12 сент. 2026')).toBeTruthy()
    expect(screen.queryByText(/язык/)).toBeNull()
    expect(screen.queryByText('Автопродление')).toBeNull()
    expect(screen.queryByText('Откуда пришёл')).toBeNull()
    expect(screen.getByText('Срок до').nextSibling?.textContent).toBe('20 янв. 2027')
    expect(screen.getByText('Статус', { selector: 'dt' }).nextSibling?.textContent).toBe('активна')
    expect(screen.getByText('Оплачено всего').nextSibling?.textContent).toBe('600 ₽')
    expect(screen.getByText('Средний чек').nextSibling?.textContent).toBe('300 ₽')
    expect(screen.getByText('Неоплаченные авансы').nextSibling?.textContent).toBe('1')
    expect(screen.getByText('Первая оплата').nextSibling?.textContent).toBe('15 сент. 2026')
    expect(screen.getByText('Последний визит').nextSibling?.textContent).toBe('—')
    expect(screen.getByText('Тариф', { selector: 'dt' }).nextSibling?.textContent).toBe('—')
    expect(screen.getByText('Промокоды', { selector: 'dt' }).nextSibling?.textContent).toBe('—')
    expect(screen.getByText('Получил рассылок').nextSibling?.textContent).toBe('0')
    expect(screen.getByText('Пригласил друзей').nextSibling?.textContent).toBe('—')
    expect(screen.getByText('Устройства').nextSibling?.textContent).toBe('—')
    const row = within(screen.getByRole('table')).getByText('31').closest('tr')!
    expect(within(row).getByText('3 мес.')).toBeTruthy()
    expect(within(row).getByText('Оплачено')).toBeTruthy()
  })

  it('shows name, tariff, last visit, broadcasts and promos from the API', async () => {
    const bob = user(7, 'bob', 2, { name: 'Боб М', tariffMonths: 6, lastSeenAt: '2026-10-05T10:00:00Z', broadcastsReceived: 12, blockedBotAt: '2026-11-01T10:00:00Z' })
    stub({ '/api/users/7': () => json(200, detail({ user: bob, promos: [{ code: 'SUMMER', kind: 'percent', value: 10, activatedAt: '' }, { code: 'DAYS5', kind: 'days', value: 5, activatedAt: '' }] })) })
    await renderAt('/users/7')
    expect(screen.getByRole('heading', { level: 2, name: 'Боб М' })).toBeTruthy()
    expect(screen.getByText('Тариф', { selector: 'dt' }).nextSibling?.textContent).toBe('6 мес.')
    expect(screen.getByText('Последний визит').nextSibling?.textContent).toMatch(/^5 окт.*13:00$/)
    expect(screen.getByText('Получил рассылок').nextSibling?.textContent).toBe('12')
    expect(screen.getByText('Промокоды', { selector: 'dt' }).nextSibling?.textContent).toBe('SUMMER, DAYS5')
    expect(screen.getByText('заблокировал бота')).toBeTruthy()
  })

  it('shows the panel ID with a copy action', async () => {
    const writeText = vi.fn(async () => {})
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    stub({ '/api/users/7': () => json(200, detail()) })
    await renderAt('/users/7')
    expect(screen.getByText('ID в панели').nextSibling?.textContent).toBe('1234')
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать ID в панели' }))
    await flush()
    expect(writeText).toHaveBeenCalledWith('1234')
    expect(screen.getByText('ID скопирован')).toBeTruthy()
  })

  it('has no t.me link without username and names the user by ID', async () => {
    stub({ '/api/users/7': () => json(200, detail({ user: user(7, '') })) })
    await renderAt('/users/7')
    expect(screen.queryByRole('link', { name: /@/ })).toBeNull()
    expect(screen.getByRole('heading', { level: 2, name: 'ID 7' })).toBeTruthy()
  })

  it.each([
    ['DISABLED', 'отключена'],
    ['not_found', 'нет в панели'],
  ])('shows panel status %s in words', async (status, label) => {
    stub({ '/api/users/7': () => json(200, detail({ panel: { id: null, expireAt: null, status, error: null } })) })
    await renderAt('/users/7')
    expect(screen.getAllByText(label)).toHaveLength(2) // тег в шапке и строка «Статус»
    expect(screen.getByText('Срок до').nextSibling?.textContent).toBe('—')
    expect(screen.getByText('ID в панели').nextSibling?.textContent).toBe('—')
  })

  it('says the panel is unavailable', async () => {
    stub({ '/api/users/7': () => json(200, detail({ panel: { id: null, expireAt: null, status: null, error: 'panel_unavailable' } })) })
    await renderAt('/users/7')
    expect(screen.getAllByText('панель недоступна')).toHaveLength(2) // тег в шапке и «Срок до»
    expect(screen.getByText('Статус', { selector: 'dt' }).nextSibling?.textContent).toBe('—')
    expect(screen.getByText('Оплачено всего')).toBeTruthy()
  })

  it('direct composer prepends the sent message and closes', async () => {
    stub({ '/api/users/7': () => json(200, detail()) })
    await renderAt('/users/7')
    expect(screen.getByText('Сообщений пока нет')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Написать' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Послать' }))
    expect(screen.getByText('привет')).toBeTruthy()
    expect(screen.queryByText('Сообщений пока нет')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Послать' })).toBeNull()
    expect(screen.getByText('Сообщение отправлено')).toBeTruthy()
  })

  it('«Написать» twice hides the composer but keeps it mounted', async () => {
    stub({ '/api/users/7': () => json(200, detail()) })
    await renderAt('/users/7')
    const write = screen.getByRole('button', { name: 'Написать' })
    fireEvent.click(write)
    const composer = await screen.findByRole('button', { name: 'Послать' })
    fireEvent.click(write)
    expect(screen.queryByRole('button', { name: 'Послать' })).toBeNull()
    expect(screen.getByText('Послать')).toBe(composer)
    expect(write.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(write)
    expect(screen.getByRole('button', { name: 'Послать' })).toBe(composer)
  })

  it('shows «Выдать дни» and «Заблокировать» as not wired', async () => {
    stub({ '/api/users/7': () => json(200, detail()) })
    await renderAt('/users/7')
    fireEvent.click(screen.getByRole('button', { name: 'Заблокировать' }))
    expect(screen.getByText('Пока не подключено')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Выдать дни' })).toBeTruthy()
  })
})
