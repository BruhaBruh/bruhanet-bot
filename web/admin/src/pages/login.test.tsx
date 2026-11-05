import { act, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@/ui'
import { setUnauthorizedHandler } from '../api'
import { BOT_USERNAME, OWNER_USERNAME } from '../brand'
import LoginRequired from './LoginRequired'

const POLL_MS = 3000
const tick = (ms = POLL_MS) => act(() => vi.advanceTimersByTimeAsync(ms))

function setup() {
  return render(
    <ToastProvider>
      <MemoryRouter initialEntries={['/login-required']}>
        <Routes>
          <Route path="/" element={<p>главная</p>} />
          <Route path="/login-required" element={<LoginRequired />} />
        </Routes>
      </MemoryRouter>
    </ToastProvider>,
  )
}

const status = (code: number) => vi.fn(async () => new Response('{}', { status: code }))

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  setUnauthorizedHandler(() => {})
})

describe('LoginRequired', () => {
  it('renders the macro: steps, bot deep link, sample reply', () => {
    vi.stubGlobal('fetch', status(401))
    setup()
    expect(screen.getByRole('heading', { level: 1, name: 'Войдите через Telegram' })).toBeTruthy()
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
    expect(screen.getByRole('link', { name: 'Открыть бота в Telegram' }).getAttribute('href')).toBe(
      `https://t.me/${BOT_USERNAME}?start=login`,
    )
    expect(screen.getByRole('link', { name: `@${BOT_USERNAME}` }).getAttribute('href')).toBe(`https://t.me/${BOT_USERNAME}`)
    expect(screen.getByRole('link', { name: `@${OWNER_USERNAME}` }).getAttribute('href')).toBe(
      `https://t.me/${OWNER_USERNAME}`,
    )
    expect(screen.getByText(new RegExp(`^Привет, @${OWNER_USERNAME}!`))).toBeTruthy()
    expect(document.querySelector('.app-root')).toBeNull()
  })

  it('opens the bot in a new tab: this one keeps polling', () => {
    vi.stubGlobal('fetch', status(401))
    setup()
    expect(screen.getByRole('link', { name: 'Открыть бота в Telegram' }).getAttribute('target')).toBe('_blank')
  })

  it('goes to / once /api/me answers 200', async () => {
    const fetchMock = status(200)
    vi.stubGlobal('fetch', fetchMock)
    setup()
    expect(fetchMock).not.toHaveBeenCalled()
    await tick()
    expect(fetchMock).toHaveBeenCalledWith('/api/me', expect.anything())
    expect(screen.getByText('главная')).toBeTruthy()
    await tick(POLL_MS * 3)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('401 keeps waiting and never fires the global handler', async () => {
    const fetchMock = status(401)
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    vi.stubGlobal('fetch', fetchMock)
    setup()
    await tick()
    await tick()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(onUnauthorized).not.toHaveBeenCalled()
    expect(screen.getByText('Ждём подтверждения из Telegram…')).toBeTruthy()
  })

  it('survives a network error', async () => {
    const fetchMock = vi.fn(async () => {
      throw new TypeError('offline')
    })
    vi.stubGlobal('fetch', fetchMock)
    setup()
    await tick()
    await tick()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('stops the timer on unmount', async () => {
    const fetchMock = status(401)
    vi.stubGlobal('fetch', fetchMock)
    const view = setup()
    await tick()
    view.unmount()
    await tick(POLL_MS * 3)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not schedule again when the answer arrives after unmount', async () => {
    let answer!: (r: Response) => void
    const fetchMock = vi.fn(() => new Promise<Response>((r) => (answer = r)))
    vi.stubGlobal('fetch', fetchMock)
    const view = setup()
    await tick()
    view.unmount()
    await act(async () => answer(new Response('{}', { status: 401 })))
    await tick(POLL_MS * 3)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('copies /login: button state and toast', async () => {
    vi.stubGlobal('fetch', status(401))
    const writeText = vi.fn(async () => {})
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    setup()
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать' }))
    await tick(0)
    expect(writeText).toHaveBeenCalledWith('/login')
    expect(screen.getByRole('button', { name: 'Скопировано' })).toBeTruthy()
    expect(screen.getByText('Команда /login скопирована')).toBeTruthy()
    await tick(2500)
    expect(screen.getByRole('button', { name: 'Скопировать' })).toBeTruthy()
  })

  it('does not claim a copy that failed', async () => {
    vi.stubGlobal('fetch', status(401))
    vi.stubGlobal('navigator', { ...navigator, clipboard: undefined })
    setup()
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать' }))
    await tick(0)
    expect(screen.getByRole('button', { name: 'Скопировать' })).toBeTruthy()
    expect(screen.getByText('Не удалось скопировать')).toBeTruthy()
  })
})
