import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation, useParams } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import BroadcastEdit from './BroadcastEdit'
import BroadcastNew from './BroadcastNew'
import Broadcasts from './Broadcasts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })

const item = (id: string, over = {}) => ({
  id,
  status: 'sent',
  audience: 'all',
  audienceDays: null,
  silent: false,
  buttons: [],
  scheduledAt: null,
  updatedAt: '2026-11-04T11:40:00Z',
  text: `<b>Заголовок ${id}</b>\nтекст`,
  total: 10,
  sent: 8,
  failed: 2,
  createdAt: '2026-11-04T11:32:00Z',
  finishedAt: '2026-11-04T11:40:00Z',
  ...over,
})

// Ответы по точному URL; `1` — сокращение для первой страницы без фильтра.
function stub(pages: Record<string, unknown>, extra: (url: string, init: RequestInit) => Response | undefined = () => undefined) {
  const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
    const r = extra(url, init)
    if (r) return r
    const key = url.replace('/api/broadcasts?page=', '')
    return pages[key] ? json(200, pages[key]) : json(500, {})
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function EditProbe() {
  return <pre data-testid="edit">{useParams().id}</pre>
}

function Probe() {
  const { state } = useLocation()
  return <pre data-testid="state">{JSON.stringify(state)}</pre>
}

async function renderList(newRoute = <Probe />) {
  render(
    <ToastProvider>
      <MemoryRouter initialEntries={['/broadcasts']}>
        <Routes>
          <Route path="/broadcasts" element={<Broadcasts />} />
          <Route path="/broadcasts/new" element={newRoute} />
          <Route path="/broadcasts/:id" element={<EditProbe />} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  )
  await flush()
}

const table = () => within(screen.getByRole('table'))

describe('Broadcasts', () => {
  it('shows a tag per status, the audience and the delivery counters', async () => {
    // Дата в ожиданиях — 7 нояб.: без заморозки часов тест ломается в сам этот день («сегодня»).
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-11-05T12:00:00Z'))
    stub({
      1: {
        items: [
          item('a'),
          item('b', { status: 'sending' }),
          item('c', { status: 'draft', total: 0, sent: 0, failed: 0 }),
          item('d', { status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z', total: 0 }),
          item('e', { status: 'cancelled', total: 0 }),
        ],
        total: 5,
      },
    })
    await renderList()
    const rows = table().getAllByRole('row').slice(1)
    for (const [i, tag] of ['Отправлена', 'Отправляется', 'Черновик', 'Запланирована', 'Отменена'].entries()) {
      expect(within(rows[i]).getByText(tag), tag).toBeTruthy()
    }
    expect(within(rows[0]).getByText('Заголовок a')).toBeTruthy()
    expect(within(rows[0]).getByText('доставлено 8 из 10, не доставлено 2')).toBeTruthy()
    expect(within(rows[0]).getByText('Все пользователи')).toBeTruthy()
    expect(within(rows[2]).getByText('черновик, не отправлялась')).toBeTruthy()
    expect(within(rows[3]).getByText(/^уйдёт 7 нояб\., 18:30 \(МСК\)$/)).toBeTruthy()
    expect(within(rows[3]).getByText('7 нояб., 18:30')).toBeTruthy() // колонка «Отправка» — время старта
  })

  it('tabs ask the server for their status and restart from page 1', async () => {
    const fetchMock = stub({
      1: { items: [item('a')], total: 1 },
      '1&status=scheduled': { items: [item('s', { status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z' })], total: 1 },
      '1&status=sending,sent': { items: [item('a')], total: 1 },
      '1&status=draft': { items: [], total: 0 },
    })
    await renderList()
    fireEvent.click(screen.getByRole('radio', { name: 'Запланированные' }))
    await flush()
    expect(table().getByText('Заголовок s')).toBeTruthy()
    fireEvent.click(screen.getByRole('radio', { name: 'Отправленные' }))
    await flush()
    fireEvent.click(screen.getByRole('radio', { name: 'Черновики' }))
    await flush()
    expect(table().getByText('Черновиков нет')).toBeTruthy()
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/broadcasts?page=1',
      '/api/broadcasts?page=1&status=scheduled',
      '/api/broadcasts?page=1&status=sending,sent',
      '/api/broadcasts?page=1&status=draft',
    ])
  })

  it('a tab keeps its page number out of the next one', async () => {
    const fetchMock = stub({
      1: { items: [item('a')], total: 41 },
      2: { items: [item('z')], total: 41 },
      '1&status=draft': { items: [], total: 0 },
    })
    await renderList()
    fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
    await flush()
    fireEvent.click(screen.getByRole('radio', { name: 'Черновики' }))
    await flush()
    expect(fetchMock.mock.calls.at(-1)?.[0]).toBe('/api/broadcasts?page=1&status=draft')
  })

  it('no «Открыли» column; «Кнопки» shows the real count', async () => {
    stub({ 1: { items: [item('a', { buttons: [{ text: 'a', url: 'https://e.com', style: '' }, { text: 'b', url: 'https://e.org', style: '' }] }), item('b')], total: 2 } })
    await renderList()
    expect(table().getAllByRole('columnheader').map((h) => h.textContent)).not.toContain('Открыли')
    const idx = table().getAllByRole('columnheader').findIndex((h) => h.textContent === 'Кнопки')
    expect(idx).toBeGreaterThan(0)
    const rows = table().getAllByRole('row').slice(1)
    expect(within(rows[0]).getAllByRole('cell')[idx].textContent).toBe('2')
    expect(within(rows[1]).getAllByRole('cell')[idx].textContent).toBe('—')
  })

  it('«Фильтры» opens the period; it goes to the server and back to page 1', async () => {
    const fetchMock = stub({
      1: { items: [item('a')], total: 1 },
      '1&from=2026-11-01': { items: [], total: 0 },
      '1&from=2026-11-01&to=2026-11-03': { items: [item('p')], total: 1 },
      '1&to=2026-11-03': { items: [item('q')], total: 1 },
    })
    await renderList()
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    fireEvent.change(screen.getByLabelText('С даты'), { target: { value: '2026-11-01' } })
    await flush()
    expect(table().getByText('Рассылок ещё не было')).toBeTruthy()
    fireEvent.change(screen.getByLabelText('По дату'), { target: { value: '2026-11-03' } })
    await flush()
    expect(table().getByText('Заголовок p')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Сбросить период' }))
    await flush()
    expect(table().getByText('Заголовок a')).toBeTruthy()
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([
      '/api/broadcasts?page=1',
      '/api/broadcasts?page=1&from=2026-11-01',
      '/api/broadcasts?page=1&from=2026-11-01&to=2026-11-03',
      '/api/broadcasts?page=1',
    ])
  })

  it('a click on a sent row opens its screen, a draft goes straight to the editor', async () => {
    stub({ 1: { items: [item('a'), item('d', { status: 'draft' })], total: 2 } })
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/broadcasts']}>
          <Routes>
            <Route path="/broadcasts" element={<Broadcasts />} />
            <Route path="/broadcasts/:id/edit" element={<p>редактор</p>} />
            <Route path="/broadcasts/:id" element={<EditProbe />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    )
    await flush()
    fireEvent.click(table().getByText('Заголовок d'))
    await flush()
    expect(screen.getByText('редактор')).toBeTruthy()
  })

  it('a click on a sent row opens its screen', async () => {
    stub({ 1: { items: [item('a')], total: 1 } })
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/broadcasts']}>
          <Routes>
            <Route path="/broadcasts" element={<Broadcasts />} />
            <Route path="/broadcasts/:id" element={<EditProbe />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    )
    await flush()
    fireEvent.click(table().getByText('Заголовок a'))
    await flush()
    expect(screen.getByTestId('edit').textContent).toBe('a')
  })

  it('pagination loads the next page via ?page=', async () => {
    const fetchMock = stub({
      1: { items: [item('a')], total: 21 },
      2: { items: [item('z')], total: 21 },
    })
    await renderList()
    expect(screen.getByText(/^1 из 2$/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Следующая страница' }))
    await flush()
    expect(fetchMock.mock.calls.at(-1)?.[0]).toBe('/api/broadcasts?page=2')
    expect(table().getByText('Заголовок z')).toBeTruthy()
  })

  it('duplicate opens the new broadcast prefilled through router state', async () => {
    stub({ 1: { items: [item('a', { photo: 'file9' })], total: 1 } })
    await renderList()
    fireEvent.click(table().getByRole('button', { name: 'Дублировать' }))
    await flush()
    expect(JSON.parse(screen.getByTestId('state').textContent!)).toEqual({ text: item('a').text, photo: 'file9', audience: 'all', audienceDays: null, silent: false, buttons: [] })
    expect(screen.getByRole('status').textContent).toContain('Копия «Заголовок a» открыта в редакторе')
  })

  it('duplicate of a broadcast with a stored picture carries photoId', async () => {
    stub({ 1: { items: [item('a', { photoId: 'pic1' })], total: 1 } })
    await renderList()
    fireEvent.click(table().getByRole('button', { name: 'Дублировать' }))
    await flush()
    expect(JSON.parse(screen.getByTestId('state').textContent!)).toMatchObject({ photoId: 'pic1' })
  })

  it('failed load is shown', async () => {
    stub({})
    await renderList()
    expect(table().getByText('Не удалось загрузить рассылки')).toBeTruthy()
  })

  it('duplicated draft is sent, toast is shown and the list opens again', async () => {
    let posted: unknown
    const fetchMock = stub({ 1: { items: [item('a')], total: 1 } }, (url, init) => {
      if (url === '/api/broadcasts/recipients?days=7') return json(200, { all: 3, active: 1, no_subscription: 1, expiring: 1 })
      if (url === '/api/broadcasts/recipients?audience=all') return json(200, { count: 3 })
      if (url === '/api/broadcasts' && init.method === 'POST') {
        posted = JSON.parse(String(init.body))
        return json(201, { broadcast: item('n', { total: 3 }) })
      }
      return undefined
    })
    await renderList(<BroadcastNew />)
    fireEvent.click(table().getByRole('button', { name: 'Дублировать' }))
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Отправить 3 получателям' }))
    await flush()
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Отправить' }))
    await flush()
    await flush()
    expect(posted).toMatchObject({ text: item('a').text })
    expect(screen.getByRole('status').textContent).toContain('Рассылка запущена — 3 получателя')
    expect(screen.getByTestId('edit').textContent).toBe('n')
    expect(fetchMock).toHaveBeenCalled()
  })
})

describe('Broadcasts delete', () => {
  const menuOf = (title: string) => within(table().getByText(title).closest('tr')!).queryByRole('button', { name: 'Действия с рассылкой' })

  it('only draft, scheduled and cancelled have «Удалить»; sent and sending do not', async () => {
    stub({
      1: {
        items: [
          item('a'),
          item('b', { status: 'sending' }),
          item('c', { status: 'draft' }),
          item('d', { status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z' }),
          item('e', { status: 'cancelled' }),
        ],
        total: 5,
      },
    })
    await renderList()
    expect([menuOf('Заголовок a'), menuOf('Заголовок b')]).toEqual([null, null])
    for (const t of ['Заголовок c', 'Заголовок d', 'Заголовок e']) expect(menuOf(t), t).toBeTruthy()
  })

  it('confirms, sends one DELETE even on a double click, toasts and reloads the list', async () => {
    let deleted = false
    const fetchMock = stub({ 1: { items: [item('s', { status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z' })], total: 1 } }, (url, init) => {
      if (init?.method === 'DELETE') {
        deleted = true
        return new Response(null, { status: 204 })
      }
      return deleted && url === '/api/broadcasts?page=1' ? json(200, { items: [], total: 0 }) : undefined
    })
    await renderList()
    fireEvent.keyDown(menuOf('Заголовок s')!, { key: 'Enter' })
    await flush()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Удалить' }))
    const d = screen.getByRole('alertdialog')
    expect(d.textContent).toContain('Удалить рассылку?')
    expect(d.textContent).toContain('Она не отправится.')
    const ok = within(d).getByRole('button', { name: 'Удалить' })
    fireEvent.click(ok)
    fireEvent.click(ok)
    await flush()
    const dels = fetchMock.mock.calls.filter((c) => c[1]?.method === 'DELETE')
    expect(dels.map((c) => c[0])).toEqual(['/api/broadcasts/s'])
    expect(screen.getByText('Рассылка удалена')).toBeTruthy()
    expect(screen.getAllByText('Рассылок ещё не было').length).toBeGreaterThan(0)
  })

  it('«Назад» deletes nothing', async () => {
    const fetchMock = stub({ 1: { items: [item('c', { status: 'draft' })], total: 1 } })
    await renderList()
    fireEvent.keyDown(menuOf('Заголовок c')!, { key: 'Enter' })
    await flush()
    fireEvent.click(screen.getByRole('menuitem', { name: 'Удалить' }))
    expect(screen.getByRole('alertdialog').textContent).not.toContain('Она не отправится')
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Назад' }))
    await flush()
    expect(fetchMock.mock.calls.some((c) => c[1]?.method === 'DELETE')).toBe(false)
  })
})

describe('BroadcastEdit', () => {
  async function openEdit(b: unknown) {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url === '/api/broadcasts/b1') return json(200, { broadcast: b })
        if (url.startsWith('/api/broadcasts/recipients')) return json(200, { all: 3, active: 1, no_subscription: 1, expiring: 1 })
        if (url === '/api/texts') return json(200, { items: [], globals: [] })
        return json(500, {})
      }),
    )
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/broadcasts/b1/edit']}>
          <Routes>
            <Route path="/broadcasts/:id/edit" element={<BroadcastEdit />} />
            <Route path="/broadcasts/:id" element={<p>экран рассылки</p>} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    )
    await flush()
    await flush()
  }

  it('a scheduled broadcast opens with its time and the save button', async () => {
    await openEdit(item('b1', { id: 'b1', status: 'scheduled', scheduledAt: '2099-01-02T07:30:00Z', silent: true, audience: 'active' }))
    expect((screen.getByLabelText('Дата и время отправки') as HTMLInputElement).value).toBe('2099-01-02T10:30')
    expect(screen.getByRole('button', { name: 'Сохранить изменения' })).toBeTruthy()
    expect(screen.getByRole('switch', { name: 'Без звука' }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('radio', { name: 'Активные подписчики' }).getAttribute('aria-checked')).toBe('true')
  })

  it('a draft opens as a normal composer with autosave line', async () => {
    await openEdit(item('b1', { id: 'b1', status: 'draft' }))
    expect(screen.getByText('Черновик сохраняется автоматически')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Сохранить изменения' })).toBeNull()
  })

  it('«Удалить» in the editor deletes and returns to the list', async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') return new Response(null, { status: 204 })
      if (url === '/api/broadcasts/b1') return json(200, { broadcast: item('b1', { id: 'b1', status: 'draft' }) })
      if (url.startsWith('/api/broadcasts/recipients')) return json(200, { all: 3, active: 1, no_subscription: 1, expiring: 1 })
      if (url === '/api/texts') return json(200, { items: [], globals: [] })
      return json(500, {})
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/broadcasts/b1/edit']}>
          <Routes>
            <Route path="/broadcasts/:id/edit" element={<BroadcastEdit />} />
            <Route path="/broadcasts" element={<p>список рассылок</p>} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    )
    await flush()
    await flush()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить' }))
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Удалить' }))
    await flush()
    expect(fetchMock.mock.calls.filter((c) => c[1]?.method === 'DELETE').map((c) => c[0])).toEqual(['/api/broadcasts/b1'])
    expect(screen.getByText('список рассылок')).toBeTruthy()
    expect(screen.getByText('Рассылка удалена')).toBeTruthy()
  })

  it('an already started broadcast goes to its screen', async () => {
    await openEdit(item('b1', { id: 'b1', status: 'sending' }))
    expect(screen.getByText('экран рассылки')).toBeTruthy()
  })
})
