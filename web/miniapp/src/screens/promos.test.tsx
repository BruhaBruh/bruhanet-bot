import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Promos from './Promos'

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })
const settle = () => act(async () => {})

const LIST = {
  items: [
    { code: 'WELCOME', kind: 'days', value: 7, activatedAt: '2026-11-01T10:00:00Z' },
    { code: 'SALE10', kind: 'percent', value: 10, activatedAt: '2026-10-20T10:00:00Z' },
    { code: 'MINUS100', kind: 'rub', value: 100, activatedAt: '2026-10-10T10:00:00Z' },
  ],
}

function stub(activate: () => Response = () => json({ addedDays: 7, expireAt: '2026-12-01T12:00:00Z' })) {
  const m = vi.fn(async (url: string, init?: RequestInit) => (url === '/m/api/promos/activate' && init?.method === 'POST' ? activate() : json(LIST)))
  vi.stubGlobal('fetch', m)
  return m
}
const posts = (m: ReturnType<typeof stub>) => m.mock.calls.filter(([u]) => u === '/m/api/promos/activate')

afterEach(() => vi.unstubAllGlobals())

const type = (value: string) => fireEvent.change(screen.getByPlaceholderText('Промокод'), { target: { value } })

describe('Promos screen', () => {
  it('lists the used codes with what they gave', async () => {
    stub()
    render(<Promos />)
    await settle()
    expect(screen.getByText('WELCOME')).toBeTruthy()
    expect(screen.getByText('+7 дней')).toBeTruthy()
    expect(screen.getByText('скидка 10%')).toBeTruthy()
    expect(screen.getByText('скидка 100 ₽')).toBeTruthy()
  })

  it('activates a code, shows the result and sends it once on a double tap', async () => {
    const m = stub()
    render(<Promos />)
    await settle()
    const button = screen.getByRole('button', { name: 'Активировать' })
    expect((button as HTMLButtonElement).disabled).toBe(true)
    type('  welcome ')
    fireEvent.click(button)
    fireEvent.click(button)
    await settle()
    expect(posts(m)).toHaveLength(1)
    expect(JSON.parse((posts(m)[0][1] as RequestInit).body as string)).toEqual({ code: 'welcome' })
    expect(screen.getByText(/Добавлено 7 дней/)).toBeTruthy()
    expect((screen.getByPlaceholderText('Промокод') as HTMLInputElement).value).toBe('')
  })

  it.each([
    [422, { error: 'promo_unavailable', message: 'Код не найден или не подходит' }, 'Код не найден или не подходит'],
    [429, { error: 'rate_limited', message: 'Слишком часто, попробуй через минуту' }, 'Слишком часто, попробуй через минуту'],
    [409, { error: 'no_subscription', message: 'Сначала оформи подписку в боте' }, 'Сначала оформи подписку в боте'],
    [502, { error: 'panel_unavailable', message: 'Панель не ответила, повтори позже' }, 'Панель не ответила, повтори позже'],
  ])('shows the server message on %i', async (status, body, text) => {
    stub(() => json(body, status))
    render(<Promos />)
    await settle()
    type('CODE')
    fireEvent.click(screen.getByRole('button', { name: 'Активировать' }))
    await settle()
    expect(screen.getByText(text)).toBeTruthy()
  })

  it('falls back to a generic text when the server sent none', async () => {
    stub(() => json({}, 500))
    render(<Promos />)
    await settle()
    type('CODE')
    fireEvent.click(screen.getByRole('button', { name: 'Активировать' }))
    await settle()
    expect(screen.getByText('Не получилось. Попробуй ещё раз.')).toBeTruthy()
  })
})
