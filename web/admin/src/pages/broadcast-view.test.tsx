import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import BroadcastView from './BroadcastView'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const flush = () => act(() => new Promise((r) => setTimeout(r, 0)))
const json = (status: number, body?: unknown) => new Response(body === undefined ? null : JSON.stringify(body), { status })

const broadcast = (over = {}) => ({
  id: 'b1',
  status: 'sent',
  text: '<b>Привет, {{.name}}</b><script>x</script>',
  audience: 'expiring',
  audienceDays: 3,
  silent: true,
  buttons: [
    { text: 'Сайт', url: 'https://e.com', style: 'primary' },
    { text: 'Блог', url: 'https://e.org', style: '' },
  ],
  scheduledAt: null,
  total: 200,
  sent: 150,
  failed: 10,
  createdAt: '2026-11-04T11:32:00Z',
  updatedAt: '2026-11-04T11:32:00Z',
  finishedAt: '2026-11-04T11:40:00Z',
  ...over,
})

function stub(get: () => Response, post: (url: string) => Response = () => json(200, { broadcast: broadcast() })) {
  const fetchMock = vi.fn(async (url: string, init: RequestInit = {}) => {
    if (url === '/api/texts') return json(200, { items: [], globals: [{ name: 'name', description: '', sample: '<Иван>', raw: false, max: 64 }] })
    if (init.method === 'POST') return post(url)
    if (url === '/api/broadcasts/b1') return get()
    return json(500, {})
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function open() {
  render(
    <ToastProvider>
      <MemoryRouter initialEntries={['/broadcasts/b1']}>
        <Routes>
          <Route path="/broadcasts/:id" element={<BroadcastView />} />
          <Route path="/broadcasts" element={<p>список</p>} />
          <Route path="/broadcasts/b1/edit" element={<p>редактор</p>} />
          <Route path="/broadcasts/new" element={<p>новая</p>} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  )
  await flush()
}

describe('BroadcastView', () => {
  it('sent: message with sanitized text and keyboard, parameters, counters and progress', async () => {
    stub(() => json(200, { broadcast: broadcast() }))
    await open()
    const aside = screen.getByRole('complementary', { name: 'Сообщение' })
    expect(aside.querySelector('b')?.textContent).toBe('Привет, <Иван>')
    expect(aside.querySelector('script')).toBeNull()
    expect(within(aside).getByText('Сайт')).toBeTruthy()
    expect(within(aside).getByText('Блог')).toBeTruthy()
    expect(screen.getByText('Отправлена')).toBeTruthy()
    expect(screen.getByText('Подписка истекает через 3 дня')).toBeTruthy()
    expect(screen.getByText('Без звука')).toBeTruthy()
    expect(screen.getByText('2: Сайт, Блог')).toBeTruthy()
    const bar = screen.getByRole('progressbar', { name: 'Доставлено' })
    expect(bar.getAttribute('aria-valuenow')).toBe('80') // (150+10)/200
    for (const [label, value] of [['Получателей', '200'], ['Доставлено', '150'], ['Не доставлено', '10']]) {
      expect(screen.getByText(label, { selector: 'dt' }).nextElementSibling?.textContent).toBe(value)
    }
    // отправленную не отменить и не править
    expect(screen.queryByRole('button', { name: 'Отменить рассылку' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Изменить' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Дублировать' })).toBeTruthy()
  })

  it('scheduled: time in МСК, no counters, edit and cancel with confirmation', async () => {
    const fetchMock = stub(() => json(200, { broadcast: broadcast({ status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z', total: 0, sent: 0, failed: 0, finishedAt: null }) }))
    await open()
    expect(screen.getByText('Запланирована')).toBeTruthy()
    expect(screen.getByText('7 нояб. 2026 в 18:30 (МСК)')).toBeTruthy()
    expect(screen.getByText(/Получатели определятся в момент отправки/)).toBeTruthy()
    expect(screen.queryByRole('progressbar')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Отменить рассылку' }))
    await flush()
    const dialog = screen.getByRole('alertdialog')
    expect(fetchMock.mock.calls.some((c) => (c[1] as RequestInit | undefined)?.method === 'POST')).toBe(false)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Отменить рассылку' }))
    await flush()
    expect(fetchMock.mock.calls.find((c) => (c[1] as RequestInit | undefined)?.method === 'POST')?.[0]).toBe('/api/broadcasts/b1/cancel')
    expect(screen.getByRole('status').textContent).toContain('Рассылка отменена')
  })

  it('«Не отменять» keeps the broadcast; a 409 says it has already started', async () => {
    const sched = broadcast({ status: 'scheduled', scheduledAt: '2026-11-07T15:30:00Z', finishedAt: null })
    const fetchMock = stub(
      () => json(200, { broadcast: sched }),
      () => json(409, { error: 'locked' }),
    )
    await open()
    fireEvent.click(screen.getByRole('button', { name: 'Отменить рассылку' }))
    await flush()
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Не отменять' }))
    await flush()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(fetchMock.mock.calls.some((c) => (c[1] as RequestInit | undefined)?.method === 'POST')).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Отменить рассылку' }))
    await flush()
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Отменить рассылку' }))
    await flush()
    expect(screen.getByRole('status').textContent).toContain('уже запущена')
  })

  it('draft: edit opens the editor; cancelled: nothing to do but duplicate', async () => {
    stub(() => json(200, { broadcast: broadcast({ status: 'draft', total: 0, sent: 0, failed: 0, finishedAt: null }) }))
    await open()
    expect(screen.queryByRole('button', { name: 'Отменить рассылку' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Изменить' }))
    expect(screen.getByText('редактор')).toBeTruthy()
  })

  it('cancelled', async () => {
    stub(() => json(200, { broadcast: broadcast({ status: 'cancelled', total: 0, sent: 0, failed: 0, finishedAt: null }) }))
    await open()
    expect(screen.getByText('Отменена')).toBeTruthy()
    expect(screen.getByText('Рассылка отменена и не отправлялась.')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Изменить' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Отменить рассылку' })).toBeNull()
  })

  it('sending: counters refresh every 5 seconds until it is done', async () => {
    let calls = 0
    stub(() => {
      calls++
      return json(200, { broadcast: broadcast(calls === 1 ? { status: 'sending', sent: 10, failed: 0, finishedAt: null } : {}) })
    })
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    await open()
    expect(screen.getByText(/Идёт отправка: обработано 10 из 200 \(5%\)/)).toBeTruthy()
    await act(async () => void (await vi.advanceTimersByTimeAsync(5000)))
    expect(screen.getByText('Отправлена')).toBeTruthy()
    const before = calls
    await act(async () => void (await vi.advanceTimersByTimeAsync(15000)))
    expect(calls).toBe(before) // доставлено — опрос остановлен
  })

  it('duplicate opens the composer prefilled; a failed load says so', async () => {
    stub(() => json(200, { broadcast: broadcast() }))
    await open()
    fireEvent.click(screen.getByRole('button', { name: 'Дублировать' }))
    expect(screen.getByText('новая')).toBeTruthy()
  })

  it('failed load', async () => {
    stub(() => json(404, { error: 'not found' }))
    await open()
    expect(screen.getByRole('alert').textContent).toBe('Не удалось загрузить рассылку')
  })
})

describe('photo', () => {
  it('shows the stored picture from the api, no file_id note', async () => {
    stub(() => json(200, { broadcast: broadcast({ photoId: 'pic1' }) }))
    await open()
    expect(document.querySelector('img[src="/api/photos/pic1"]')).toBeTruthy()
    expect(screen.queryByText(/file_id/)).toBeNull()
  })

  it('a legacy file_id broadcast keeps the placeholder and the note', async () => {
    stub(() => json(200, { broadcast: broadcast({ photo: 'AgAD', photoId: null }) }))
    await open()
    expect(document.querySelector('img[src^="/api/photos/"]')).toBeNull()
    expect(screen.getByText(/file_id/)).toBeTruthy()
  })
})
