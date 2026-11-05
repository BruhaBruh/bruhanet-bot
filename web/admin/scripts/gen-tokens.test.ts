// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { generate } from './gen-tokens.mjs'

const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')
const css = generate(JSON.parse(read('../design/tokens.json')))

// Тело блока по точному селектору: тема не должна протекать в соседний блок.
function block(selector: string) {
  const start = css.indexOf(`${selector} {`)
  expect(start, selector).toBeGreaterThanOrEqual(0)
  return css.slice(start, css.indexOf('}', start))
}

describe('gen-tokens', () => {
  const light = block(':root,\n[data-theme="light"]')
  const dark = block('[data-theme="dark"]')

  it('привязывает --tg-theme-* с запасным hex своей темы', () => {
    expect(light).toContain('--bg: var(--tg-theme-bg-color, #ffffff);')
    expect(dark).toContain('--bg: var(--tg-theme-bg-color, #000000);')
    expect(dark).toContain('--surface: var(--tg-theme-section-bg-color, var(--tg-theme-bg-color, #2c2c2e));')
    expect(dark).toContain('--header-bg: var(--tg-theme-header-bg-color, #1a1a1af0);')
  })

  it('в тёмном блоке нет светлых запасных значений', () => {
    expect(dark).not.toContain('#efeff4')
    expect(dark).not.toContain('#007aff)')
  })

  it('tint акцента следует за button_color и хранит прозрачность своей темы', () => {
    expect(light).toContain('--accent-tint: color-mix(in srgb, var(--tg-theme-button-color, #007aff) 12%, transparent);')
    expect(dark).toContain('--accent-tint: color-mix(in srgb, var(--tg-theme-button-color, #3e88f7) 20%, transparent);')
  })

  it('токены без аналога в themeParams — простые значения', () => {
    expect(light).toContain('--success: #34c759;')
    expect(dark).toContain('--success: #30d158;')
    expect(dark).toContain('--destructive-tint: #eb554533;')
    expect(dark).toContain('--avatar-red: #ff516a;')
    expect(dark).toContain('--shadow-card: none;')
  })

  it('раскрывает алиасы {token}', () => {
    expect(light).toContain('--switch-on: var(--success);')
    expect(dark).toContain('--switch-on: var(--success);')
  })

  it('отдаёт отступы, радиусы, размеры, шрифты и классы Tailwind', () => {
    expect(css).toContain('--space-4: 16px;')
    expect(css).toContain('--radius-md: 10px;')
    expect(css).toContain('--size-sidebar: 248px;')
    expect(css).toContain('--font-mono: ui-monospace,')
    expect(css).toContain('--color-text-secondary: var(--text-secondary);')
    expect(css).toContain('--radius-bubble: var(--radius-bubble);')
    expect(css).toMatch(/@utility admin-label \{\s+font-family: var\(--font-sans\);\s+font-size: 13px;\s+line-height: 16px;\s+font-weight: 600;/)
    expect(css).toMatch(/@utility mono \{\s+font-family: var\(--font-mono\);/)
    expect(css).toMatch(/@utility admin-stat \{[^}]+letter-spacing: -0\.5px;[^}]+tabular-nums/)
  })

  it('совпадает с закоммиченным tokens.css (иначе pnpm tokens)', () => {
    expect(read('../src/styles/tokens.css')).toBe(css)
  })
})
