import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DataTable, ToastProvider } from '@/ui'
import PromoNew from './PromoNew'
import Promos from './Promos'

type Call = { url: string; method: string; body: Record<string, unknown> | undefined }
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
const debounce = () => act(() => new Promise((r) => setTimeout(r, 320)))

async function show(at = '/promos') {
  render(
    <ToastProvider>
      <MemoryRouter initialEntries={[at]}>
        <Routes>
          <Route path="/promos" element={<Promos />} />
          <Route path="/promos/new" element={<PromoNew />} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  )
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

const table = () => within(screen.getByRole('table'))
const row = (code: string) => table().getByText(code).closest('tr')!
const segment = (name: string) => fireEvent.click(screen.getByRole('radio', { name }))
const posts = () => calls.filter((c) => c.method === 'POST')

async function openMenu(code: string) {
  fireEvent.keyDown(within(row(code)).getByRole('button', { name: 'Действия с промокодом' }), { key: 'Enter' })
  await flush()
}

describe('Promos list', () => {
  it('renders the columns as in the macro and computes status', async () => {
    replies['GET /api/promos?status=all'] = json(200, {
      items: [
        promo(1, 'SUMMER', { maxActivations: 5, used: 2, validUntil: '2099-12-31', onlyNew: true }),
        promo(2, 'WEEK', { kind: 'days', value: 7, telegramId: 7, username: 'ann' }),
        promo(3, 'FULL', { kind: 'rub', value: 1500, maxActivations: 2, used: 2, telegramId: 8 }),
        promo(4, 'OLD', { validUntil: '2020-01-31' }),
        promo(5, 'OFF', { disabled: true }),
      ],
    })
    await show()
    const cells = (code: string) => Array.from(row(code).querySelectorAll('td')).map((td) => td.textContent)
    expect(cells('SUMMER').slice(0, 7)).toEqual(['SUMMER', 'Скидка в %', '−10%', 'Новые пользователи', '2 / 5', '31 дек. 2099', 'Активен'])
    expect(cells('WEEK').slice(1, 7)).toEqual(['Бонусные дни', '+7 дн.', '@ann', '0 / ∞', 'бессрочно', 'Активен'])
    expect(cells('FULL').slice(2, 4)).toEqual(['−1 500 ₽', 'ID 8'])
    expect(cells('FULL')[6]).toBe('Лимит исчерпан')
    expect(cells('OLD')[6]).toBe('Истёк')
    expect(cells('OFF')[6]).toBe('Выключен')
  })

  it('segments: active asks the API, expired filters on the client, drafts are empty', async () => {
    replies['GET /api/promos?status=all'] = json(200, { items: [promo(1, 'LIVE'), promo(2, 'OLD', { validUntil: '2020-01-31' })] })
    replies['GET /api/promos?status=active'] = json(200, { items: [promo(1, 'LIVE')] })
    await show()
    segment('Истёкшие')
    await flush()
    expect(table().queryByText('LIVE')).toBeNull()
    expect(table().getByText('OLD')).toBeTruthy()
    expect(calls.filter((c) => c.url === '/api/promos?status=all')).toHaveLength(1)
    segment('Активные')
    await flush()
    expect(table().getByText('LIVE')).toBeTruthy()
    expect(table().queryByText('OLD')).toBeNull()
    segment('Черновики')
    await flush()
    expect(table().getByText('Черновиков нет')).toBeTruthy()
  })

  it('stale filter response does not overwrite newer', async () => {
    const all = deferred()
    replies['GET /api/promos?status=all'] = () => all.p
    replies['GET /api/promos?status=active'] = json(200, { items: [promo(2, 'NEWER')] })
    await show()
    segment('Активные')
    await flush()
    expect(table().getByText('NEWER')).toBeTruthy()
    all.resolve(json(200, { items: [promo(1, 'OLDER')] })())
    await flush()
    expect(table().queryByText('OLDER')).toBeNull()
    expect(table().getByText('NEWER')).toBeTruthy()
  })

  it('shows a load error', async () => {
    await show()
    expect(table().getByText('Не удалось загрузить промокоды')).toBeTruthy()
  })

  it('row click expands activations, action buttons do not', async () => {
    replies['GET /api/promos?status=all'] = json(200, { items: [promo(1, 'SUMMER')] })
    replies['GET /api/promos/1/activations'] = json(200, {
      items: [{ id: 3, promoId: 1, telegramId: 7, username: 'ann', paymentId: 12, createdAt: '2026-11-02T10:00:00Z' }],
    })
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => {}) } })
    await show()
    fireEvent.click(within(row('SUMMER')).getByRole('button', { name: 'Скопировать код' }))
    await flush()
    expect(row('SUMMER').getAttribute('aria-expanded')).toBe('false')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('SUMMER')
    expect(screen.getByRole('status').textContent).toContain('Код SUMMER скопирован')

    fireEvent.click(row('SUMMER'))
    await flush()
    expect(row('SUMMER').getAttribute('aria-expanded')).toBe('true')
    expect(table().getByText(/@ann · .* · платёж #12/)).toBeTruthy()
    fireEvent.click(row('SUMMER'))
    expect(table().queryByText(/@ann/)).toBeNull()
  })

  it('disable asks in a dialog, posts once and reloads the current filter', async () => {
    replies['GET /api/promos?status=all'] = json(200, { items: [promo(1, 'ACTIVE'), promo(2, 'GONE', { disabled: true })] })
    replies['GET /api/promos?status=active'] = json(200, { items: [promo(1, 'ACTIVE')] })
    const disabled = deferred()
    replies['POST /api/promos/1/disable'] = () => disabled.p
    await show()

    await openMenu('GONE')
    expect(screen.getByRole('menuitem', { name: 'Выключить' }).getAttribute('aria-disabled')).toBe('true')
    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
    await flush()

    await openMenu('ACTIVE')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Выключить' }))
    await flush()
    const dialog = within(screen.getByRole('alertdialog'))
    expect(posts()).toHaveLength(0)
    const confirm = dialog.getByRole('button', { name: 'Выключить' })
    fireEvent.click(confirm)
    await flush()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(posts()).toHaveLength(1)
    // фильтр сменили, пока POST в полёте
    segment('Активные')
    await flush()
    disabled.resolve(json(204)())
    await flush()
    expect(calls.at(-1)!.url).toBe('/api/promos?status=active')
    expect(table().queryByText('GONE')).toBeNull()
    expect(screen.getByRole('status').textContent).toContain('Промокод ACTIVE выключен')
  })

  it('cancel in the disable dialog sends nothing and returns focus to the menu button', async () => {
    replies['GET /api/promos?status=all'] = json(200, { items: [promo(1, 'ACTIVE')] })
    await show()
    await openMenu('ACTIVE')
    fireEvent.click(screen.getByRole('menuitem', { name: 'Выключить' }))
    await flush()
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Отмена' }))
    await flush()
    expect(posts()).toHaveLength(0)
    expect(document.activeElement).toBe(within(row('ACTIVE')).getByRole('button', { name: 'Действия с промокодом' }))
  })
})

describe('DataTable expandedRow', () => {
  it('shows the panel under the row and in the mobile list', () => {
    render(
      <MemoryRouter>
        <DataTable
          columns={[{ key: 'code', title: 'Код' }]}
          rows={[{ id: 1, code: 'A' }, { id: 2, code: 'B' }]}
          onRowClick={() => {}}
          mobileRow={(r) => ({ title: r.code })}
          expandedRow={(r) => (r.id === 1 ? <p>панель {r.code}</p> : null)}
        />
      </MemoryRouter>,
    )
    const t = within(screen.getByRole('table'))
    expect(t.getByText('A').closest('tr')!.getAttribute('aria-expanded')).toBe('true')
    expect(t.getByText('B').closest('tr')!.getAttribute('aria-expanded')).toBe('false')
    // следующая строка таблицы — панель на все колонки
    expect(t.getByText('A').closest('tr')!.nextElementSibling!.textContent).toBe('панель A')
    // таблица и мобильный список
    expect(screen.getAllByText('панель A')).toHaveLength(2)
    const mobile = screen.getAllByRole('button', { name: 'A' }).at(-1)!
    expect(mobile.getAttribute('aria-expanded')).toBe('true')
    expect(mobile.nextElementSibling!.textContent).toBe('панель A')
  })
})

describe('PromoNew', () => {
  const create = () => screen.getByRole('button', { name: 'Создать промокод' }) as HTMLButtonElement
  const type = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } })

  it('double click sends one POST with an empty code, then toast and back to the list', async () => {
    const created = deferred()
    replies['POST /api/promos'] = () => created.p
    replies['GET /api/promos?status=all'] = json(200, { items: [promo(5, 'X7K2')] })
    await show('/promos/new')
    expect(create().disabled).toBe(true)
    type('Размер скидки', '15')
    const btn = create()
    fireEvent.click(btn)
    fireEvent.click(btn)
    await flush()
    expect(posts()).toHaveLength(1)
    expect(posts()[0].body).toEqual({ code: '', kind: 'percent', value: 15, maxActivations: null, validUntil: null, onlyNew: false, user: '' })
    created.resolve(json(201, { promo: promo(5, 'X7K2') })())
    await flush()
    expect(screen.getByRole('status').textContent).toContain('Промокод X7K2 создан')
    expect(table().getByText('X7K2')).toBeTruthy()
  })

  it('Enter submits with the same guards, other buttons do not', async () => {
    replies['POST /api/promos'] = json(201, { promo: promo(5, 'X7K2') })
    replies['GET /api/promos?status=all'] = json(200, { items: [] })
    await show('/promos/new')
    const form = create().closest('form')!
    fireEvent.submit(form)
    await flush()
    expect(posts()).toHaveLength(0)
    type('Размер скидки', '15')
    fireEvent.click(screen.getByRole('button', { name: 'Сгенерировать' }))
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить срок действия' }))
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить срок действия' }))
    segment('Бонусные дни')
    await flush()
    expect(posts()).toHaveLength(0)
    fireEvent.submit(form)
    fireEvent.submit(form)
    await flush()
    expect(posts()).toHaveLength(1)
  })

  it('summary pluralizes the activation limit', async () => {
    await show('/promos/new')
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить количество активаций' }))
    type('Сколько раз можно применить', '2')
    expect(within(screen.getByRole('complementary', { name: 'Итог' })).getByText('2 раза')).toBeTruthy()
  })

  it('shows server 400 and 409 messages under the form', async () => {
    replies['POST /api/promos'] = json(409, { error: 'exists', message: 'Такой код уже есть' })
    await show('/promos/new')
    type('Размер скидки', '15')
    fireEvent.click(create())
    await flush()
    expect(screen.getByRole('alert').textContent).toBe('Такой код уже есть')
    replies['POST /api/promos'] = json(400, { error: 'invalid', message: 'дата уже прошла' })
    fireEvent.click(create())
    await flush()
    expect(screen.getByRole('alert').textContent).toBe('дата уже прошла')
  })

  it('local validation does not reach the server', async () => {
    await show('/promos/new')
    type('Код', 'плохой код')
    type('Размер скидки', '15')
    fireEvent.click(create())
    await flush()
    expect(screen.getByRole('alert').textContent).toContain('Код — 3–32 символа')
    expect(posts()).toHaveLength(0)
  })

  it('percent out of range blocks creation', async () => {
    await show('/promos/new')
    type('Размер скидки', '101')
    expect(screen.getByText('От 1 до 100%')).toBeTruthy()
    expect(create().disabled).toBe(true)
  })

  it('generates a code and switches kind with macro defaults', async () => {
    await show('/promos/new')
    fireEvent.click(screen.getByRole('button', { name: 'Сгенерировать' }))
    expect((screen.getByLabelText('Код') as HTMLInputElement).value).toMatch(/^[A-Z2-9]{8}$/)
    segment('Бонусные дни')
    expect((screen.getByLabelText('Сколько дней подписки добавить') as HTMLInputElement).value).toBe('7')
    expect(within(screen.getByRole('complementary', { name: 'Итог' })).getByText(/^\+7\sдн\. подписки$/)).toBeTruthy()
  })

  it('sends limit and validUntil as an inclusive date, requires them when switched on', async () => {
    replies['POST /api/promos'] = json(201, { promo: promo(5, 'SALE') })
    replies['GET /api/promos?status=all'] = json(200, { items: [] })
    await show('/promos/new')
    type('Код', 'sale')
    type('Размер скидки', '15')
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить количество активаций' }))
    expect(create().disabled).toBe(true)
    type('Сколько раз можно применить', '100')
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить срок действия' }))
    expect(create().disabled).toBe(true)
    expect((screen.getByLabelText('Действует с') as HTMLInputElement).disabled).toBe(true)
    type('Действует по', '2099-12-31')
    expect(within(screen.getByRole('complementary', { name: 'Итог' })).getByText('по 31 дек. 2099')).toBeTruthy()
    fireEvent.click(create())
    await flush()
    expect(posts()[0].body).toMatchObject({ code: 'SALE', maxActivations: 100, validUntil: '2099-12-31', user: '' })
  })

  it('switching term off drops the entered date', async () => {
    replies['POST /api/promos'] = json(201, { promo: promo(5, 'SALE') })
    replies['GET /api/promos?status=all'] = json(200, { items: [] })
    await show('/promos/new')
    type('Размер скидки', '15')
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить срок действия' }))
    type('Действует по', '2099-12-31')
    fireEvent.click(screen.getByRole('switch', { name: 'Ограничить срок действия' }))
    fireEvent.click(create())
    await flush()
    expect(posts()[0].body).toMatchObject({ validUntil: null })
  })

  it('new users target sends onlyNew', async () => {
    replies['POST /api/promos'] = json(201, { promo: promo(5, 'SALE') })
    replies['GET /api/promos?status=all'] = json(200, { items: [] })
    await show('/promos/new')
    type('Размер скидки', '15')
    segment('Для новых')
    fireEvent.click(create())
    await flush()
    expect(posts()[0].body).toMatchObject({ onlyNew: true, user: '' })
  })

  it('user search: stale answer ignored, pick sends telegram id, «Изменить» goes back', async () => {
    const user = (telegramId: number, username: string) => ({ telegramId, username, createdAt: '2026-01-01T00:00:00Z', confirmed: 0, confirmedSum: 0 })
    const slow = deferred()
    replies['GET /api/users?q='] = json(200, { items: [] })
    replies['GET /api/users?q=a'] = () => slow.p
    replies['GET /api/users?q=%40bo'] = json(200, { items: [user(42, 'bob'), user(43, '')] })
    replies['POST /api/promos'] = json(201, { promo: promo(5, 'SALE') })
    replies['GET /api/promos?status=all'] = json(200, { items: [] })
    await show('/promos/new')
    type('Размер скидки', '15')
    segment('Конкретному пользователю')
    expect(create().disabled).toBe(true)
    await debounce()
    expect(screen.getByText('Никого не нашли')).toBeTruthy()

    const search = screen.getByRole('searchbox')
    fireEvent.change(search, { target: { value: 'a' } })
    await debounce()
    fireEvent.change(search, { target: { value: '@bo' } })
    await debounce()
    slow.resolve(json(200, { items: [user(1, 'stale')] })())
    await flush()
    expect(screen.queryByText('@stale')).toBeNull()
    expect(screen.getByText('ID 43')).toBeTruthy()

    fireEvent.click(screen.getByText('@bob').closest('button')!)
    expect(screen.queryByRole('searchbox')).toBeNull()
    expect(screen.getByText('@bob · ID 42')).toBeTruthy()
    expect(within(screen.getByRole('complementary', { name: 'Итог' })).getByText('@bob')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Изменить' }))
    expect(screen.getByRole('searchbox')).toBeTruthy()
    expect(create().disabled).toBe(true)
    await debounce()
    replies['GET /api/users?q=%40bo'] = json(200, { items: [user(42, 'bob')] })
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '@bo' } })
    await debounce()
    fireEvent.click(screen.getByText('@bob').closest('button')!)
    fireEvent.click(create())
    await flush()
    expect(posts()[0].body).toMatchObject({ user: '42', onlyNew: false })
  })
})
