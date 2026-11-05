import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import BroadcastNew from './BroadcastNew'
import Broadcasts from './Broadcasts'

afterEach(() => vi.unstubAllGlobals())

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })

const item = (id: string, over = {}) => ({
  id,
  text: `<b>Заголовок ${id}</b>\nтекст`,
  total: 10,
  sent: 8,
  failed: 2,
  createdAt: '2026-11-04T11:32:00Z',
  finishedAt: '2026-11-04T11:40:00Z',
  ...over,
})

function stub(pages: Record<string, unknown>, extra: (url: string, init: RequestInit) => Response | undefined = () => undefined) {
  const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
    const r = extra(url, init)
    if (r) return r
    const key = url.split('page=')[1]
    return key && pages[key] ? json(200, pages[key]) : json(500, {})
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
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
  it('maps status: finishedAt → Отправлено, otherwise Отправляется', async () => {
    stub({ 1: { items: [item('a'), item('b', { finishedAt: null })], total: 2 } })
    await renderList()
    const rows = table().getAllByRole('row').slice(1)
    expect(within(rows[0]).getByText('Отправлено')).toBeTruthy()
    expect(within(rows[1]).getByText('Отправляется')).toBeTruthy()
    expect(within(rows[0]).getByText('Заголовок a')).toBeTruthy()
    expect(within(rows[0]).getByText('доставлено 8 из 10, не доставлено 2')).toBeTruthy()
    expect(within(rows[0]).getByText('Все пользователи')).toBeTruthy()
  })

  it('segment «Отправленные» hides those still sending; scheduled and drafts are empty', async () => {
    stub({ 1: { items: [item('a'), item('b', { finishedAt: null })], total: 2 } })
    await renderList()
    fireEvent.click(screen.getByRole('radio', { name: 'Отправленные' }))
    expect(table().queryByText('Заголовок b')).toBeNull()
    expect(table().getByText('Заголовок a')).toBeTruthy()
    fireEvent.click(screen.getByRole('radio', { name: 'Запланированные' }))
    expect(table().getByText('Запланированных рассылок нет')).toBeTruthy()
    expect(screen.queryByText(/Показано/)).toBeNull()
    fireEvent.click(screen.getByRole('radio', { name: 'Черновики' }))
    expect(table().getByText('Черновиков нет')).toBeTruthy()
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
    expect(JSON.parse(screen.getByTestId('state').textContent!)).toEqual({ text: item('a').text, photo: 'file9' })
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
      if (url === '/api/broadcasts/recipients') return json(200, { count: 3 })
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
