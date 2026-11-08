import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Friends from './Friends'

const openTelegramLink = vi.fn()
vi.mock('~/lib/telegram', async (orig) => ({ ...(await orig<typeof import('~/lib/telegram')>()), shareLink: (url: string, text: string) => openTelegramLink(url, text) }))

const REF = { enabled: true, link: 'https://t.me/bruhanet_bot?start=ref_42', days: 7, invited: 5, paid: 2, rewardDays: 14 }
const writeText = vi.fn(async () => {})
const settle = () => act(async () => {})

beforeEach(() => {
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(REF))))
})
afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('Friends screen', () => {
  it('shows the stats and the reward rule', async () => {
    render(<Friends />)
    await settle()
    expect(screen.getByText('Приглашено')).toBeTruthy()
    expect(screen.getByText('5')).toBeTruthy()
    expect(screen.getByText('2')).toBeTruthy()
    expect(screen.getByText('14 д.')).toBeTruthy()
    expect(screen.getByText(/по 7 д\./)).toBeTruthy()
  })

  it('copies and shares the personal link', async () => {
    render(<Friends />)
    await settle()
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать ссылку' }))
    await settle()
    expect(writeText).toHaveBeenCalledWith(REF.link)
    fireEvent.click(screen.getByRole('button', { name: 'Поделиться' }))
    expect(openTelegramLink).toHaveBeenCalledWith(REF.link, expect.any(String))
  })
})
