import { act, render, screen } from '@testing-library/react'
import { StrictMode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TgLogin from './TgLogin'

const INIT = 'query_id=AAH&user=%7B%22id%22%3A1%7D&auth_date=1&hash=abc'

function setup(hash: string) {
  window.history.replaceState(null, '', `/tg${hash}`)
  return render(
    <MemoryRouter initialEntries={['/tg']}>
      <Routes>
        <Route path="/tg" element={<TgLogin />} />
        <Route path="/" element={<p>главная</p>} />
        <Route path="/login-required" element={<p>нужен вход</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

const flush = () => act(() => vi.advanceTimersByTimeAsync(0))

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
  window.history.replaceState(null, '', '/')
})

describe('TgLogin', () => {
  it('posts the signed initData from the hash and opens the admin on 204', async () => {
    vi.useFakeTimers()
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    setup(`#tgWebAppData=${encodeURIComponent(INIT)}&tgWebAppVersion=9.0`)
    await flush()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/auth/tma')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('same-origin')
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(JSON.parse(init.body as string)).toEqual({ initData: INIT })
    expect(screen.getByText('главная')).toBeTruthy()
    expect(window.location.hash).toBe('') // подпись не остаётся в адресе и истории
  })

  it('falls back to the login page when the server refuses', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 401 })))
    setup(`#tgWebAppData=${encodeURIComponent(INIT)}`)
    await flush()
    expect(screen.getByText('нужен вход')).toBeTruthy()
  })

  it('without initData does not call the server at all', async () => {
    vi.useFakeTimers()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    setup('')
    await flush()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(screen.getByText('нужен вход')).toBeTruthy()
  })
})

describe('TgLogin under StrictMode', () => {
  it('sends the signature once: a second effect run must not turn into a refusal', async () => {
    vi.useFakeTimers()
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    window.history.replaceState(null, '', `/tg#tgWebAppData=${encodeURIComponent(INIT)}`)
    render(
      <StrictMode>
        <MemoryRouter initialEntries={['/tg']}>
          <Routes>
            <Route path="/tg" element={<TgLogin />} />
            <Route path="/" element={<p>главная</p>} />
            <Route path="/login-required" element={<p>нужен вход</p>} />
          </Routes>
        </MemoryRouter>
      </StrictMode>,
    )
    await flush()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(screen.getByText('главная')).toBeTruthy()
  })
})
