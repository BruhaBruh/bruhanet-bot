import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, post } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
  delete window.Telegram
})

const stubTelegram = (initData: string) => {
  window.Telegram = { WebApp: { initData } as NonNullable<Window['Telegram']>['WebApp'] }
}

describe('api', () => {
  it('signs every request with the initData and never sends cookies', async () => {
    stubTelegram('query_id=AAH&hash=abc')
    const fetchMock = vi.fn(async () => new Response('{"ok":true}'))
    vi.stubGlobal('fetch', fetchMock)
    await api('/m/api/me')
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect((init.headers as Record<string, string>).Authorization).toBe('tma query_id=AAH&hash=abc')
    expect(init.credentials).toBe('omit')
    expect(init.cache).toBe('no-store')
    expect((init.headers as Record<string, string>)['Content-Type']).toBeUndefined()
  })

  it('sends mutations as json', async () => {
    stubTelegram('x')
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    await post('/m/api/promos/activate', { code: 'A' })
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(init.body).toBe('{"code":"A"}')
  })

  it('turns an error answer into ApiError with the server code and message', async () => {
    stubTelegram('x')
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"promo_unavailable","message":"Код не найден"}', { status: 422 })))
    const err = await api('/m/api/promos/activate').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 422, code: 'promo_unavailable', serverMessage: 'Код не найден' })
  })

  it('keeps working when the body of an error is not json', async () => {
    stubTelegram('x')
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>', { status: 502 })))
    await expect(api('/m/api/me')).rejects.toMatchObject({ status: 502, code: 'error' })
  })
})
