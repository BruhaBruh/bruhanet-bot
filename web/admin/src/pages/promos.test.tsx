import { AppRoot } from '@telegram-apps/telegram-ui'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Promos from './Promos'

type Call = { url: string; method: string; body: any }
let calls: Call[]
let replies: Record<string, () => Response | Promise<Response>>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  calls = []
  replies = {}
  vi.stubGlobal('fetch', async (url: string, init: RequestInit = {}) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return (replies[`${method} ${url}`] ?? json(500))()
  })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const show = async () => {
  render(<Promos />, { wrapper: AppRoot })
  await flush()
}

const promo = (id: number, code: string, extra: object = {}) => ({
  id,
  code,
  kind: 'percent',
  value: 10,
  maxActivations: null,
  used: 0,
  validUntil: null,
  onlyNew: false,
  telegramId: null,
  username: null,
  disabled: false,
  createdAt: '2026-11-01T10:00:00Z',
  ...extra,
})

// ответ, который отдаём вручную
function deferred() {
  let resolve!: (r: Response) => void
  const p = new Promise<Response>((r) => (resolve = r))
  return { p, resolve }
}

describe('Promos', () => {
  it('shows promo marks and expands activations', async () => {
    replies['GET /api/promos?status=active'] = json(200, {
      items: [promo(1, 'SUMMER', { maxActivations: 5, used: 2, validUntil: '2026-12-31', onlyNew: true, telegramId: 7, username: null })],
    })
    replies['GET /api/promos/1/activations'] = json(200, {
      items: [{ id: 3, promoId: 1, telegramId: 7, username: 'ann', paymentId: 12, createdAt: '2026-11-02T10:00:00Z' }],
    })
    await show()
    expect(screen.getByText('−10% к оплате · использовано 2 / 5 · до 31.12 · только новым · ID 7')).toBeTruthy()
    const row = screen.getByText('SUMMER').closest('[aria-expanded]')!
    expect(row.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(row)
    await flush()
    expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByText(/@ann · .* · платёж #12/)).toBeTruthy()
  })

  it('stale filter response does not overwrite newer', async () => {
    const active = deferred()
    replies['GET /api/promos?status=active'] = () => active.p
    replies['GET /api/promos?status=all'] = json(200, { items: [promo(2, 'NEWER')] })
    await show()
    fireEvent.click(screen.getByText('Все'))
    await flush()
    expect(screen.getByText('NEWER')).toBeTruthy()
    active.resolve(json(200, { items: [promo(1, 'OLDER')] })())
    await flush()
    expect(screen.queryByText('OLDER')).toBeNull()
    expect(screen.getByText('NEWER')).toBeTruthy()
  })

  it('double click on create sends one POST', async () => {
    replies['GET /api/promos?status=active'] = json(200, { items: [] })
    const created = deferred()
    replies['POST /api/promos'] = () => created.p
    await show()
    fireEvent.change(screen.getByLabelText('Значение'), { target: { value: '15' } })
    const btn = screen.getByText('Создать').closest('button')!
    fireEvent.click(btn)
    fireEvent.click(btn)
    await flush()
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(1)
    expect(calls.find((c) => c.method === 'POST')!.body).toMatchObject({ kind: 'percent', value: 15, code: '' })
    created.resolve(json(201, { promo: promo(5, 'X7K2') })())
    await flush()
    expect(screen.getByText('Создан X7K2')).toBeTruthy()
  })

  it('shows server error under the form', async () => {
    replies['GET /api/promos?status=active'] = json(200, { items: [] })
    replies['POST /api/promos'] = json(409, { error: 'exists', message: 'Такой код уже есть' })
    await show()
    fireEvent.change(screen.getByLabelText('Значение'), { target: { value: '15' } })
    fireEvent.click(screen.getByText('Создать'))
    await flush()
    expect(screen.getByRole('alert').textContent).toBe('Такой код уже есть')
  })
})
