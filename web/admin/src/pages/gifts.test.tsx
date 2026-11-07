import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import { MeProvider, NO_ME } from '../me'
import Gifts from './Gifts'

type Call = { url: string; method: string; body: Record<string, unknown> | undefined }
let calls: Call[]
let replies: Record<string, () => Response | Promise<Response>>

const json = (status: number, body?: unknown) => () =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

beforeEach(() => {
  calls = []
  replies = { 'GET /api/settings': json(200, { prices: { '1': 150, '6': 800 } }) }
  vi.stubGlobal('fetch', async (url: string, init: RequestInit = {}) => {
    const method = init.method ?? 'GET'
    calls.push({ url, method, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return (replies[`${method} ${url}`] ?? json(500))()
  })
  vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => {}) } })
})
afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))

async function show(username = 'bruhabot') {
  render(
    <ToastProvider>
      <MeProvider value={{ ...NO_ME, bot: { username, name: 'Bot' } }}>
        <MemoryRouter>
          <Gifts />
        </MemoryRouter>
      </MeProvider>
    </ToastProvider>,
  )
  await flush()
}

const gift = (id: number, extra: object = {}) => ({
  id,
  code: `CODE${id}xxxxxxxxxxxxxxxxx`,
  months: 3,
  status: 'ready',
  buyerId: null,
  buyerUsername: '',
  paymentId: null,
  byAdmin: true,
  redeemedBy: null,
  redeemedUsername: '',
  redeemedAt: null,
  createdAt: '2026-11-01T10:00:00Z',
  ...extra,
})

const table = () => within(screen.getByRole('table'))
const row = (text: string) => table().getByText(text).closest('tr')!
const segment = (name: string) => fireEvent.click(screen.getByRole('radio', { name }))
const posts = () => calls.filter((c) => c.method === 'POST')

describe('Gifts list', () => {
  it('shows source, status and recipient; a pending gift has no code', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, {
      total: 4,
      items: [
        gift(1),
        gift(2, { byAdmin: false, buyerId: 7, buyerUsername: 'ann', months: 12 }),
        gift(3, { status: 'redeemed', redeemedBy: 9, redeemedUsername: 'bob', redeemedAt: '2026-11-02T10:00:00Z' }),
        gift(4, { status: 'pending', code: '', byAdmin: false, buyerId: 8, paymentId: 5 }),
      ],
    })
    await show()
    const cells = (text: string) => Array.from(row(text).querySelectorAll('td')).map((td) => td.textContent)
    expect(cells('CODE1x…').slice(0, 5)).toEqual(['CODE1x…', '3\u00a0месяца', 'выдан админом', 'Активен', '—'])
    expect(cells('CODE2x…').slice(1, 4)).toEqual(['12\u00a0месяцев', 'куплен @ann', 'Активен'])
    expect(cells('CODE3x…').slice(3, 5)).toEqual(['Использован', '@bob'])
    const pending = row('куплен ID 8')
    expect(pending.textContent).toContain('Ждёт оплаты')
    expect(pending.textContent).toContain('куплен ID 8')
    expect(pending.textContent).not.toContain('CODE4')
  })

  it('tabs ask the server for their status and go back to page 1', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 120, items: [gift(1)] })
    replies['GET /api/gifts?status=all&page=2'] = json(200, { total: 120, items: [gift(2)] })
    replies['GET /api/gifts?status=ready&page=1'] = json(200, { total: 1, items: [gift(1)] })
    replies['GET /api/gifts?status=redeemed&page=1'] = json(200, { total: 0, items: [] })
    replies['GET /api/gifts?status=cancelled&page=1'] = json(200, { total: 0, items: [] })
    await show()
    fireEvent.click(screen.getByRole('button', { name: /След/ }))
    await flush()
    segment('Активные')
    await flush()
    segment('Использованные')
    await flush()
    expect(screen.getAllByText('Использованных подарков нет').length).toBeGreaterThan(0)
    segment('Отозванные')
    await flush()
    expect(screen.getAllByText('Отозванных подарков нет').length).toBeGreaterThan(0)
    expect(calls.map((c) => c.url)).toEqual([
      '/api/gifts?status=all&page=1',
      '/api/gifts?status=all&page=2',
      '/api/gifts?status=ready&page=1',
      '/api/gifts?status=redeemed&page=1',
      '/api/gifts?status=cancelled&page=1',
    ])
  })

  it('copies the one-time link made from the bot username', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    await show()
    fireEvent.click(within(row('CODE1x…')).getByRole('button', { name: 'Скопировать ссылку' }))
    await flush()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('https://t.me/bruhabot?start=gift_CODE1xxxxxxxxxxxxxxxxx')
  })

  it('without the bot username it says so instead of copying a half link', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    await show('')
    fireEvent.click(within(row('CODE1x…')).getByRole('button', { name: 'Скопировать ссылку' }))
    await flush()
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled()
    expect(screen.getByText('Имя бота неизвестно, ссылку не собрать')).toBeTruthy()
  })

  it('load failure is shown', async () => {
    await show()
    expect(screen.getAllByText('Не удалось загрузить подарки').length).toBeGreaterThan(0)
  })
})

describe('Gifts create', () => {
  const open = async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 0, items: [] })
    await show()
    fireEvent.click(screen.getByRole('button', { name: 'Создать подарок' }))
    await flush()
  }

  it('picks a tariff term, posts months and count, then lists copyable links', async () => {
    await open()
    const dialog = within(screen.getByRole('alertdialog'))
    fireEvent.click(dialog.getByRole('button', { name: '6 месяцев' }))
    fireEvent.change(dialog.getByLabelText('Сколько подарков'), { target: { value: '2' } })
    replies['POST /api/gifts'] = json(201, { items: [gift(1, { months: 6 }), gift(2, { months: 6 })] })
    fireEvent.click(dialog.getByRole('button', { name: 'Создать' }))
    await flush()
    expect(posts().map((c) => c.body)).toEqual([{ months: 6, count: 2 }])
    const links = within(screen.getByRole('alertdialog'))
    expect(links.getByText('https://t.me/bruhabot?start=gift_CODE1xxxxxxxxxxxxxxxxx')).toBeTruthy()
    expect(links.getByText('https://t.me/bruhabot?start=gift_CODE2xxxxxxxxxxxxxxxxx')).toBeTruthy()
    fireEvent.click(links.getByRole('button', { name: 'Скопировать ссылку 2' }))
    await flush()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('https://t.me/bruhabot?start=gift_CODE2xxxxxxxxxxxxxxxxx')
    fireEvent.click(links.getByRole('button', { name: 'Готово' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
  })

  it('does not post invalid months or count', async () => {
    await open()
    const dialog = within(screen.getByRole('alertdialog'))
    fireEvent.change(dialog.getByLabelText('Срок подарка'), { target: { value: '121' } })
    fireEvent.click(dialog.getByRole('button', { name: 'Создать' }))
    await flush()
    expect(posts()).toEqual([])
    expect(dialog.getByText('От 1 до 120')).toBeTruthy()
    fireEvent.change(dialog.getByLabelText('Срок подарка'), { target: { value: '12' } })
    fireEvent.change(dialog.getByLabelText('Сколько подарков'), { target: { value: '21' } })
    fireEvent.click(dialog.getByRole('button', { name: 'Создать' }))
    await flush()
    expect(posts()).toEqual([])
    expect(dialog.getByText('От 1 до 20')).toBeTruthy()
  })

  it('a server refusal keeps the form and toasts the reason', async () => {
    await open()
    replies['POST /api/gifts'] = json(400, { error: 'invalid', message: 'срок подарка — от 1 до 120 месяцев' })
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Создать' }))
    await flush()
    expect(screen.getByText('срок подарка — от 1 до 120 месяцев')).toBeTruthy()
    expect(screen.getByRole('alertdialog')).toBeTruthy()
  })
})

describe('Gifts revoke', () => {
  async function openMenu(text: string) {
    fireEvent.keyDown(within(row(text)).getByRole('button', { name: 'Действия с подарком' }), { key: 'Enter' })
    await flush()
  }

  it('cancels a ready gift after a confirmation and reloads the list', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    replies['POST /api/gifts/1/cancel'] = json(204)
    await show()
    await openMenu('CODE1x…')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Отозвать' }))
    const d = screen.getByRole('alertdialog')
    expect(d.textContent).toContain('Отозвать подарок?')
    expect(posts()).toEqual([])
    fireEvent.click(within(d).getByRole('button', { name: 'Отозвать подарок' }))
    await flush()
    expect(posts().map((c) => c.url)).toEqual(['/api/gifts/1/cancel'])
    expect(calls.filter((c) => c.url === '/api/gifts?status=all&page=1')).toHaveLength(2)
    expect(screen.getByText('Подарок отозван')).toBeTruthy()
  })

  it('«Назад» in the confirmation cancels nothing', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    await show()
    await openMenu('CODE1x…')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Отозвать' }))
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Назад' }))
    await flush()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(posts()).toEqual([])
  })

  it('a redeemed or pending gift has no actions; a revoked one can only be deleted', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, {
      total: 3,
      items: [gift(1, { status: 'redeemed', redeemedBy: 9 }), gift(2, { status: 'cancelled' }), gift(3, { status: 'pending', code: '' })],
    })
    await show()
    expect(screen.getAllByRole('button', { name: 'Действия с подарком' })).toHaveLength(1)
    await openMenu('CODE2x…')
    expect(screen.getAllByRole('menuitem').map((m) => m.textContent)).toEqual(['Удалить'])
  })

  it('«already used» from the server is shown and the list is refreshed', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    replies['POST /api/gifts/1/cancel'] = json(409, { error: 'not_active', message: 'Подарок уже использован или отменён' })
    await show()
    await openMenu('CODE1x…')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Отозвать' }))
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Отозвать подарок' }))
    await flush()
    expect(screen.getByText('Подарок уже использован или отменён')).toBeTruthy()
    expect(calls.filter((c) => c.url === '/api/gifts?status=all&page=1')).toHaveLength(2)
  })
})

describe('Gifts delete', () => {
  async function openMenu(text: string) {
    fireEvent.keyDown(within(row(text)).getByRole('button', { name: 'Действия с подарком' }), { key: 'Enter' })
    await flush()
  }
  const dels = () => calls.filter((c) => c.method === 'DELETE')

  it('a ready gift offers revoke and delete; the dialog says what happens; one DELETE on a double click', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    replies['DELETE /api/gifts/1'] = json(204)
    await show()
    await openMenu('CODE1x…')
    expect(screen.getAllByRole('menuitem').map((m) => m.textContent)).toEqual(['Отозвать', 'Удалить'])
    fireEvent.click(screen.getByRole('menuitem', { name: 'Удалить' }))
    const d = screen.getByRole('alertdialog')
    expect(d.textContent).toContain('Удалить подарок?')
    expect(d.textContent).toContain('Ссылка перестанет работать')
    const ok = within(d).getByRole('button', { name: 'Удалить' })
    fireEvent.click(ok)
    fireEvent.click(ok)
    await flush()
    expect(dels().map((c) => c.url)).toEqual(['/api/gifts/1'])
    expect(screen.getByText('Подарок удалён')).toBeTruthy()
    expect(calls.filter((c) => c.url === '/api/gifts?status=all&page=1')).toHaveLength(2)
  })

  it('the revoke dialog explains that a redeemed gift keeps the recipient term', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    await show()
    await openMenu('CODE1x…')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Отозвать' }))
    expect(screen.getByRole('alertdialog').textContent).toContain('срок получателя не изменится')
  })

  it('«not deletable» from the server is shown and the list is refreshed', async () => {
    replies['GET /api/gifts?status=all&page=1'] = json(200, { total: 1, items: [gift(1)] })
    replies['DELETE /api/gifts/1'] = json(409, { error: 'not_deletable' })
    await show()
    await openMenu('CODE1x…')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Удалить' }))
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Удалить' }))
    await flush()
    expect(screen.getByText('Подарок ждёт оплаты или уже использован: его не удалить')).toBeTruthy()
    expect(calls.filter((c) => c.url === '/api/gifts?status=all&page=1')).toHaveLength(2)
  })
})
