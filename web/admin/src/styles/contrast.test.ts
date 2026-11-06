// @vitest-environment node
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

// F4: активный пункт меню и текста — пара accent-strong / on-accent-strong ≥ 4.5:1 в обеих темах.
const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')
const doc = JSON.parse(read('../../design/tokens.json')) as { color: { tokens: { name: string; value: { light: string; dark: string } }[] } }
const tokens = new Map(doc.color.tokens.map((t) => [t.name, t.value]))

type Rgb = [number, number, number]
const hex = (h: string): { rgb: Rgb; a: number } => {
  const v = h.replace('#', '')
  return { rgb: [0, 2, 4].map((i) => parseInt(v.slice(i, i + 2), 16)) as Rgb, a: v.length === 8 ? parseInt(v.slice(6), 16) / 255 : 1 }
}
const color = (name: string, theme: 'light' | 'dark') => {
  const t = tokens.get(name)
  if (!t) throw new Error(`no token ${name}`)
  return hex(t[theme])
}
// цвет с прозрачностью поверх непрозрачного фона
const over = (fg: { rgb: Rgb; a: number }, bg: Rgb): Rgb => fg.rgb.map((c, i) => c * fg.a + bg[i] * (1 - fg.a)) as Rgb
const lum = ([r, g, b]: Rgb) => {
  const [R, G, B] = [r, g, b].map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * R + 0.7152 * G + 0.0722 * B
}
const ratio = (a: Rgb, b: Rgb) => {
  const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe.each(['light', 'dark'] as const)('contrast, %s theme', (theme) => {
  const fill = color('accent-strong', theme).rgb
  const onFill = color('on-accent-strong', theme).rgb

  it('active menu item and text row: on-accent-strong on accent-strong', () => {
    expect(ratio(onFill, fill)).toBeGreaterThanOrEqual(4.5)
  })

  it('the badge on an active item (inverse pair) keeps the same ratio', () => {
    expect(ratio(fill, onFill)).toBeGreaterThanOrEqual(4.5)
  })

  it('active tab of the mobile bar: accent-strong on the bar background', () => {
    const page = color('bg', theme).rgb
    const bar = over(color('bottom-bar-bg', theme), page)
    expect(ratio(fill, bar)).toBeGreaterThanOrEqual(4.5)
  })

  it('the old pair really was below the rule (the test measures something)', () => {
    if (theme === 'light') expect(ratio(color('on-accent', theme).rgb, color('accent', theme).rgb)).toBeLessThan(4.5)
  })
})

describe('active items use the tokens, not the old accent pair', () => {
  const src = (rel: string) => read(`../${rel}`)
  it('sidebar, tab bar and texts list', () => {
    expect(src('ui/sidebar.tsx')).toContain("'bg-accent-strong font-semibold text-on-accent-strong")
    expect(src('ui/sidebar.tsx')).not.toMatch(/bg-accent[^-]|text-on-accent[^-]/)
    expect(src('ui/tab-bar.tsx')).toContain("on && 'text-accent-strong'")
    expect(src('pages/Texts.tsx')).toContain("on ? 'bg-accent-strong text-on-accent-strong'")
  })
})
