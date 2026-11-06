import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation, useParams } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import BroadcastEdit from './BroadcastEdit'
import BroadcastNew from './BroadcastNew'
import Broadcasts from './Broadcasts'

afterEach(() => vi.unstubAllGlobals())

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
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  )
  await flush()
}

const table = () => within(screen.getByRole('table'))

describe('Broadcasts', () => {
  it('shows a tag per status, the audience and the delivery counters', async () => {
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

  it('a click on a draft or a scheduled row opens the editor', async () => {
    stub({ 1: { items: [item('d', { status: 'draft' }), item('s', { status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z' })], total: 2 } })
    render(
      <ToastProvider>
        <MemoryRouter initialEntries={['/broadcasts']}>
          <Routes>
            <Route path="/broadcasts" element={<Broadcasts />} />
            <Route path="/broadcasts/:id/edit" element={<EditProbe />} />
          </Routes>
        </MemoryRouter>
      </ToastProvider>,
    )
    await flush()
    fireEvent.click(table().getByText('Заголовок s'))
    await flush()
    expect(screen.getByTestId('edit').textContent).toBe('s')
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

  it('«Фильтры» and the row menu are not wired yet', async () => {
    stub({ 1: { items: [item('a')], total: 1 } })
    await renderList()
    fireEvent.click(screen.getByRole('button', { name: 'Фильтры' }))
    await flush()
    expect(screen.getByRole('status').textContent).toContain('Пока не подключено')
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
    expect(table().getByText('Заголовок a')).toBeTruthy()
    expect(fetchMock).toHaveBeenCalled()
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
            <Route path="/broadcasts" element={<p>список</p>} />
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

  it('an already started broadcast goes back to the list', async () => {
    await openEdit(item('b1', { id: 'b1', status: 'sending' }))
    expect(screen.getByText('список')).toBeTruthy()
  })
})
