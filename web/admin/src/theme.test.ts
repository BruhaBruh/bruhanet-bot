import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useTheme } from './theme'

const attr = () => document.documentElement.dataset.theme

function stubSystem(matches: boolean) {
  const listeners = new Set<() => void>()
  const mq = {
    matches,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  }
  vi.stubGlobal('matchMedia', () => mq)
  return {
    listeners,
    set(value: boolean) {
      mq.matches = value
      listeners.forEach((fn) => fn())
    },
  }
}

beforeEach(() => localStorage.clear())
afterEach(() => vi.unstubAllGlobals())

it('follows the system live while nothing is stored', () => {
  const system = stubSystem(true)
  const { result, unmount } = renderHook(() => useTheme())
  expect(result.current.theme).toBe('dark')
  expect(attr()).toBe('dark')

  act(() => system.set(false))
  expect(result.current.theme).toBe('light')
  expect(attr()).toBe('light')

  unmount()
  expect(system.listeners.size).toBe(0)
})

it('toggles, persists and then ignores the system', () => {
  const system = stubSystem(false)
  const first = renderHook(() => useTheme())
  act(() => first.result.current.toggle())
  expect(first.result.current.theme).toBe('dark')
  expect(attr()).toBe('dark')

  act(() => system.set(false))
  expect(first.result.current.theme).toBe('dark')
  first.unmount()

  const second = renderHook(() => useTheme())
  expect(second.result.current.theme).toBe('dark')
  act(() => second.result.current.toggle())
  expect(second.result.current.theme).toBe('light')
})

it('shares the theme between hook users', () => {
  stubSystem(false)
  const a = renderHook(() => useTheme())
  const b = renderHook(() => useTheme())
  act(() => a.result.current.toggle())
  expect(b.result.current.theme).toBe('dark')
})

it('Telegram colorScheme beats the stored choice and follows themeChanged', () => {
  stubSystem(false)
  localStorage.setItem('bn_theme', 'light')
  const handlers = new Set<() => void>()
  const webApp = {
    colorScheme: 'dark' as 'dark' | 'light',
    onEvent: (_: string, fn: () => void) => handlers.add(fn),
    offEvent: (_: string, fn: () => void) => handlers.delete(fn),
  }
  vi.stubGlobal('Telegram', { WebApp: webApp })

  const { result, unmount } = renderHook(() => useTheme())
  expect(result.current.theme).toBe('dark')

  act(() => {
    webApp.colorScheme = 'light'
    handlers.forEach((fn) => fn())
  })
  expect(result.current.theme).toBe('light')

  unmount()
  expect(handlers.size).toBe(0)
})
