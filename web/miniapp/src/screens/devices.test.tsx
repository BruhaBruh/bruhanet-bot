import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Devices from './Devices'

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })
const settle = () => act(async () => {})

const LIST = {
  limit: 5,
  items: [
    { hwid: 'hw/1', platform: 'iOS', osVersion: '18.1', deviceModel: 'iPhone 15', userAgent: 'Happ/2', createdAt: '2026-10-01T10:00:00Z', updatedAt: '2026-11-01T10:00:00Z' },
    { hwid: 'hw-2', platform: 'Windows', osVersion: null, deviceModel: null, userAgent: null, createdAt: '2026-10-05T10:00:00Z', updatedAt: '2026-11-02T10:00:00Z' },
  ],
}

// Панель «живая»: удаление меняет следующий ответ списка (после удаления приложение сверяется с панелью).
function stub(opts: { list?: () => Response; del?: (url: string) => Response } = {}) {
  let items = [...LIST.items]
  const m = vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') {
      const res = (opts.del ?? (() => new Response(null, { status: 204 })))(url)
      if (res.ok || res.status === 404) items = url === '/m/api/devices' ? [] : items.filter((d) => d.hwid !== decodeURIComponent(url.split('/').pop() ?? ''))
      return res
    }
    return (opts.list ?? (() => json({ ...LIST, items })))()
  })
  vi.stubGlobal('fetch', m)
  return m
}
const deletes = (m: ReturnType<typeof stub>) => m.mock.calls.filter(([, i]) => (i as RequestInit | undefined)?.method === 'DELETE').map(([u]) => u)

afterEach(() => vi.unstubAllGlobals())

describe('Devices screen', () => {
  it('lists the devices with the used/limit counter', async () => {
    stub()
    render(<Devices />)
    await settle()
    expect(screen.getByText('2 из 5')).toBeTruthy()
    expect(screen.getByText('iPhone 15')).toBeTruthy()
    expect(screen.getByText(/iOS 18\.1/)).toBeTruthy()
    expect(screen.getByText('Windows')).toBeTruthy()
  })

  it('removes one device only after the confirmation, with a single request', async () => {
    const m = stub()
    render(<Devices />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить iPhone 15' }))
    expect(deletes(m)).toHaveLength(0)
    const confirm = screen.getByRole('button', { name: 'Удалить' })
    fireEvent.click(confirm)
    fireEvent.click(confirm)
    await settle()
    expect(deletes(m)).toEqual(['/m/api/devices/hw%2F1'])
    expect(screen.queryByText('iPhone 15')).toBeNull()
    expect(screen.getByText('1 из 5')).toBeTruthy()
  })

  it('cancelling the dialog deletes nothing', async () => {
    const m = stub()
    render(<Devices />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить iPhone 15' }))
    fireEvent.click(screen.getByRole('button', { name: 'Отмена' }))
    await settle()
    expect(deletes(m)).toHaveLength(0)
    expect(screen.getByText('iPhone 15')).toBeTruthy()
  })

  it('treats 404 as already deleted', async () => {
    stub({ del: () => json({ error: 'not found' }, 404) })
    render(<Devices />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить iPhone 15' }))
    fireEvent.click(screen.getByRole('button', { name: 'Удалить' }))
    await settle()
    expect(screen.queryByText('iPhone 15')).toBeNull()
  })

  it('removes everything through one confirmed request', async () => {
    const m = stub()
    render(<Devices />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить все' }))
    fireEvent.click(screen.getByRole('button', { name: 'Удалить' }))
    await settle()
    expect(deletes(m)).toEqual(['/m/api/devices'])
    expect(screen.getByText('Устройств нет')).toBeTruthy()
  })

  it('shows an error when the panel is down and keeps the list on a failed delete', async () => {
    stub({ list: () => json({ error: 'panel_unavailable' }, 502) })
    render(<Devices />)
    await settle()
    expect(screen.getByText('Панель не отвечает')).toBeTruthy()
    vi.unstubAllGlobals()
    stub({ del: () => json({ error: 'panel_unavailable' }, 502) })
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Удалить iPhone 15' }))
    fireEvent.click(screen.getByRole('button', { name: 'Удалить' }))
    await settle()
    expect(screen.getByText('iPhone 15')).toBeTruthy()
    expect(screen.getByText(/Не получилось удалить/)).toBeTruthy()
  })
})
