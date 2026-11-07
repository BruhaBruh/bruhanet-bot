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

// Node >= 22 держит свой localStorage (без --localstorage-file он undefined), и vitest не перекрывает его jsdom-овским.
// jsdom нет в тестах с окружением node (gen-tokens).
const { jsdom } = globalThis as unknown as { jsdom?: { window: Window } }
if (jsdom) Object.defineProperty(globalThis, 'localStorage', { value: jsdom.window.localStorage, configurable: true })

// Radix Select (jsdom не умеет pointer capture и прокрутку к элементу).
if (typeof Element !== 'undefined') {
  Element.prototype.hasPointerCapture ??= () => false
  Element.prototype.setPointerCapture ??= () => {}
  Element.prototype.releasePointerCapture ??= () => {}
  Element.prototype.scrollIntoView ??= () => {}
}
