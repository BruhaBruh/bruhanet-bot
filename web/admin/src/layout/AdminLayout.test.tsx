import { act, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { get } from '../api'
import { App } from '../App'

afterEach(() => vi.unstubAllGlobals())

const renderAt = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )

describe('AdminLayout', () => {
  it('marks the active section and lists all menu items', () => {
    renderAt('/payments')
    const nav = screen.getByRole('navigation', { name: 'Разделы' })
    const links = Array.from(nav.querySelectorAll('a'))
    expect(links.map((a) => a.textContent)).toEqual([
      expect.stringContaining('Рассылки'),
      expect.stringContaining('Промокоды'),
      expect.stringContaining('Юзеры'),
      expect.stringContaining('Платежи'),
      expect.stringContaining('Тексты'),
      expect.stringContaining('Настройки'),
    ])
    expect(screen.getByRole('button', { name: 'Выйти' })).toBeTruthy()
    const active = links.filter((a) => a.getAttribute('aria-current') === 'page')
    expect(active).toHaveLength(1)
    expect(active[0].textContent).toContain('Платежи')
    expect(document.title).toBe('Платежи — BruhaNet')
  })

  it('redirects unknown paths to /users', () => {
    renderAt('/nope')
    expect(document.title).toBe('Юзеры — BruhaNet')
  })

  it('sends any 401 to /login-required', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 401 })))
    renderAt('/payments')
    await act(async () => {
      await expect(get('/api/payments')).rejects.toThrow('unauthorized')
    })
    expect(screen.getByText('Нужен вход')).toBeTruthy()
    expect(screen.queryByRole('navigation')).toBeNull()
    expect(document.title).toBe('Нужен вход — BruhaNet')
  })
})
