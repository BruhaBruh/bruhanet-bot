import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { useAppearance } from './theme'

afterEach(() => vi.unstubAllGlobals())

it('follows prefers-color-scheme and switches on change', () => {
  const listeners = new Set<() => void>()
  const mq = {
    matches: true,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  }
  vi.stubGlobal('matchMedia', (q: string) => {
    expect(q).toBe('(prefers-color-scheme: dark)')
    return mq
  })

  const { result, unmount } = renderHook(() => useAppearance())
  expect(result.current).toBe('dark')

  act(() => {
    mq.matches = false
    listeners.forEach((fn) => fn())
  })
  expect(result.current).toBe('light')

  unmount()
  expect(listeners.size).toBe(0)
})
