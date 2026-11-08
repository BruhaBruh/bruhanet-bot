import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// без globals: true RTL сам не чистит DOM между тестами
afterEach(cleanup)

// Radix меряет контент через ResizeObserver, в jsdom его нет.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// Node >= 22 держит свой localStorage, и vitest не перекрывает его jsdom-овским.
const { jsdom } = globalThis as unknown as { jsdom?: { window: Window } }
if (jsdom) Object.defineProperty(globalThis, 'localStorage', { value: jsdom.window.localStorage, configurable: true })

if (typeof Element !== 'undefined') {
  Element.prototype.hasPointerCapture ??= () => false
  Element.prototype.setPointerCapture ??= () => {}
  Element.prototype.releasePointerCapture ??= () => {}
  Element.prototype.scrollIntoView ??= () => {}
}
