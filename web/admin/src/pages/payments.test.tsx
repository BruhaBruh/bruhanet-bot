import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from '../App'
import type { Payment } from '../types'
import { extendsTo } from './payment-review'

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
    expect(table().getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['ID', 'Пользователь', 'Тариф', 'Сумма', 'Статус', 'Дата', 'Действия'])
    const cells = table().getAllByRole('cell').map((c) => c.textContent)
    expect(cells[0]).toBe('1')
    expect(cells[2]).toBe('3 мес.')
    expect(cells[3]).toBe('900 ₽')
    expect(cells[4]).toBe('Оплачено')
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
    await pick('Отклонён')
    expect(calls.at(-1)?.get('status')).toBe('declined')
    await pick('Отменён')
    expect(calls.at(-1)?.get('status')).toBe('cancelled')
    expect(screen.queryByRole('radio', { name: 'Возвраты' })).toBeNull()
  })

  it('«Фильтры» have only the period', async () => {
    stub()
    await renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    expect(screen.queryByRole('group', { name: 'Статус' })).toBeNull()
    expect(screen.queryByRole('radio', { name: 'Любой' })).toBeNull()
    expect(screen.getByLabelText('С даты')).toBeTruthy()
    expect(screen.getByLabelText('По дату')).toBeTruthy()
  })

  it('marks the advance and shows the promo code with the base price', async () => {
    stub(() => json(200, { items: [payment(1, 'declined', { advanceGranted: true, promoCode: 'SALE10', basePrice: 1000 }), payment(2)], total: 2, revenue: 0 }))
    await renderPage()
    const row = within(table().getByText('1').closest('tr')!)
    expect(row.getByText('аванс')).toBeTruthy()
    expect(row.getByText(/^SALE10 · было 1\s000\s₽$/)).toBeTruthy()
    const plain = within(table().getByText('2').closest('tr')!)
    expect(plain.queryByText('аванс')).toBeNull()
    expect(plain.queryByText(/было/)).toBeNull()
  })

  it('dates a confirmed payment by confirmation, the rest by creation', async () => {
    const dates = { createdAt: '2026-01-10T10:00:00Z', resolvedAt: '2026-02-20T10:00:00Z' }
    stub(() => json(200, { items: [payment(1, 'confirmed', dates), payment(2, 'declined', dates), payment(3, 'pending', { createdAt: dates.createdAt })], total: 3, revenue: 0 }))
    await renderPage()
    const dateOf = (id: string) => within(table().getByText(id).closest('tr')!).getAllByRole('cell')[5].textContent
    expect(dateOf('1')).toContain('20 февр.')
    expect(dateOf('2')).toContain('10 янв.')
    expect(dateOf('3')).toContain('10 янв.')
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

  describe('review', () => {
    const NOW = '2026-11-02T12:00:00Z'
    const pending = payment(1, 'pending', {
      month: 1,
      hasPhoto: true,
      submittedAt: '2026-11-01T10:00:00Z',
      prevExpireAt: '2099-12-01T12:00:00Z',
    })

    // Экран + POST: posts — адреса решений; badge — сколько раз перечитали бейдж.
    function stubReview(answer: () => Response = () => json(200, {})) {
      const posts: string[] = []
      const counters = { badge: 0, list: 0 }
      let rows = [pending]
      vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push(url)
          const res = answer()
          if (res.ok) rows = [{ ...pending, status: 'confirmed' }]
          return res
        }
        if (!url.startsWith('/api/payments')) return new Promise(() => {})
        if (url.includes('from=2000-')) {
          counters.badge++
          return json(200, { items: [], total: rows[0].status === 'pending' ? 1 : 0, revenue: 0 })
        }
        counters.list++
        return json(200, { items: rows, total: 1, revenue: 0 })
      })
      return { posts, counters }
    }
    const row = () => within(table().getByText('Подтвердить').closest('tr')!)
    const dialog = () => within(screen.getByRole('alertdialog'))
    const accept = async (name: string) => {
      fireEvent.click(dialog().getByRole('button', { name }))
      await flush()
    }

    it('extends from the current term or from now if it has passed', () => {
      const p = { month: 2, prevExpireAt: '2026-11-10T00:00:00Z' } as Payment
      expect(extendsTo(p, Date.parse('2026-11-01T00:00:00Z'))).toBe('2027-01-09T00:00:00.000Z')
      expect(extendsTo(p, Date.parse('2026-12-01T00:00:00Z'))).toBe('2027-01-30T00:00:00.000Z')
    })

    it('shows actions only on pending rows', async () => {
      stub(() => json(200, { items: [payment(1, 'confirmed', { hasPhoto: true }), payment(2, 'declined', { submittedAt: NOW }), payment(3, 'cancelled')], total: 3, revenue: 0 }))
      await renderPage()
      expect(table().queryByRole('button', { name: 'Подтвердить' })).toBeNull()
      expect(table().queryByRole('button', { name: 'Отклонить' })).toBeNull()
      expect(table().getByRole('button', { name: 'Чек' })).toBeTruthy()
      expect(table().getAllByText('Чек в боте')).toHaveLength(1)
    })

    it('confirm states the consequence, posts once, then reloads the list and the badge', async () => {
      const { posts, counters } = stubReview()
      await renderPage()
      const { badge, list } = counters
      fireEvent.click(row().getByRole('button', { name: 'Подтвердить' }))
      const text = screen.getByRole('alertdialog').textContent
      expect(text).toContain('Подтвердить платёж?')
      // prev в будущем: + 30 дней от него
      expect(text).toContain('продлится до 31 дек. 2099')
      expect(posts).toEqual([])
      await accept('Подтвердить')
      expect(posts).toEqual(['/api/payments/1/confirm'])
      expect(screen.getByText('Платёж подтверждён')).toBeTruthy()
      expect(counters.list).toBe(list + 1)
      expect(counters.badge).toBe(badge + 1)
      expect(table().queryByRole('button', { name: 'Подтвердить' })).toBeNull()
    })

    it('decline names the date the term returns to', async () => {
      const { posts } = stubReview()
      await renderPage()
      fireEvent.click(row().getByRole('button', { name: 'Отклонить' }))
      expect(screen.getByRole('alertdialog').textContent).toContain('вернётся к 1 дек. 2099')
      await accept('Отклонить')
      expect(posts).toEqual(['/api/payments/1/decline'])
      expect(screen.getByText('Платёж отклонён')).toBeTruthy()
    })

    it('cancel posts nothing', async () => {
      const { posts } = stubReview()
      await renderPage()
      fireEvent.click(row().getByRole('button', { name: 'Отклонить' }))
      await accept('Отмена')
      expect(screen.queryByRole('alertdialog')).toBeNull()
      expect(posts).toEqual([])
    })

    it('a second decision while the first is in flight sends nothing', async () => {
      let release!: () => void
      const posts: string[] = []
      vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push(url)
          await new Promise<void>((r) => (release = r))
          return json(200, {})
        }
        if (!url.startsWith('/api/payments')) return new Promise(() => {})
        return json(200, { items: [pending], total: 1, revenue: 0 })
      })
      await renderPage()
      fireEvent.click(row().getByRole('button', { name: 'Подтвердить' }))
      await accept('Подтвердить')
      fireEvent.click(row().getByRole('button', { name: 'Подтвердить' }))
      await accept('Подтвердить')
      expect(posts).toHaveLength(1)
      await act(async () => release())
      await flush()
    })

    it.each([
      [409, { error: 'already_resolved', message: 'Уже обработано' }, 'Уже обработано'],
      [404, { error: 'not_found', message: 'Платёж устарел' }, 'Платёж устарел'],
      [502, { error: 'panel_unavailable', message: 'Панель не ответила, платёж не изменён' }, 'Панель не ответила, платёж не изменён'],
      [500, { error: 'internal' }, 'Не удалось обновить платёж'],
    ])('shows the reason on %i and reloads the row', async (code, body, text) => {
      const { counters } = stubReview(() => json(code, body))
      await renderPage()
      const { list } = counters
      fireEvent.click(row().getByRole('button', { name: 'Подтвердить' }))
      await accept('Подтвердить')
      expect(screen.getByText(text)).toBeTruthy()
      expect(counters.list).toBe(list + 1)
    })

    it('opens the receipt with alt, falls back on a load error', async () => {
      stubReview()
      await renderPage()
      fireEvent.click(row().getByRole('button', { name: 'Чек' }))
      const img = screen.getByRole('img', { name: 'Чек платежа 1 от @eve' })
      expect(img.getAttribute('src')).toBe('/api/payments/1/photo')
      fireEvent.error(img)
      expect(screen.getByRole('alert').textContent).toContain('остался в боте')
    })

    it('on a phone a tap expands the row to reach the actions', async () => {
      const { posts } = stubReview()
      await renderPage()
      // мобильное дерево вне таблицы: кнопок решения там нет, пока строка не раскрыта
      expect(screen.getAllByRole('button', { name: 'Подтвердить' })).toHaveLength(1)
      const cell = screen.getAllByText('@eve').map((e) => e.closest('button')).find(Boolean)!
      expect(cell.getAttribute('aria-expanded')).toBe('false')
      fireEvent.click(cell)
      expect(cell.getAttribute('aria-expanded')).toBe('true')
      expect(screen.getAllByRole('button', { name: 'Открыть пользователя' }).length).toBeGreaterThan(0)
      fireEvent.click(screen.getAllByRole('button', { name: 'Подтвердить' }).at(-1)!)
      await accept('Подтвердить')
      expect(posts).toEqual(['/api/payments/1/confirm'])
    })
  })
})
