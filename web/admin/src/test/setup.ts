import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// без globals: true RTL сам не чистит DOM между тестами
afterEach(cleanup)

// Radix (Tooltip, Popper) меряет контент через ResizeObserver, в jsdom его нет.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
}
