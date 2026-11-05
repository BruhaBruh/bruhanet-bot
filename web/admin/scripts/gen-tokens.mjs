// design/tokens.json → src/styles/tokens.css. Запуск: pnpm tokens.
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

// Токены с аналогом в themeParams (README DS, «Привязка к теме Telegram»), цепочка var() по порядку.
// Запасное значение — hex той темы, в чьём блоке стоит привязка: один :root из README перетёр бы тёмную.
const BINDINGS = {
  bg: ['bg-color'],
  'page-bg': ['secondary-bg-color'],
  surface: ['section-bg-color', 'bg-color'],
  'surface-elevated': ['section-bg-color'],
  'header-bg': ['header-bg-color'],
  'bottom-bar-bg': ['bottom-bar-bg-color'],
  text: ['text-color'],
  'text-secondary': ['subtitle-text-color'],
  'text-hint': ['hint-color'],
  'section-header': ['section-header-text-color'],
  link: ['link-color'],
  accent: ['button-color'],
  'on-accent': ['button-text-color'],
  'accent-text': ['accent-text-color'],
  destructive: ['destructive-text-color'],
  separator: ['section-separator-color'],
}

const tgVar = (params, fallback) =>
  params.reduceRight((inner, p) => `var(--tg-theme-${p}, ${inner})`, fallback)

// #rrggbbaa → color-mix от button_color: прозрачность из тем (12% / 20%), цвет от Telegram.
function accentTint(hex) {
  const percent = Math.round((parseInt(hex.slice(7), 16) / 255) * 100)
  return `color-mix(in srgb, ${tgVar(BINDINGS.accent, hex.slice(0, 7))} ${percent}%, transparent)`
}

function colorValue(name, raw) {
  if (raw.startsWith('{')) return `var(--${raw.slice(1, -1)})`
  if (name === 'accent-tint') return accentTint(raw)
  return BINDINGS[name] ? tgVar(BINDINGS[name], raw) : raw
}

const themed = (token, theme) =>
  typeof token.value === 'string' ? token.value : token.value[theme]

const decls = (entries) => entries.map(([k, v]) => `  --${k}: ${v};`).join('\n')

const themeBlock = (selector, tokens, theme) => `${selector} {
${decls([
  ...tokens.color.tokens.map((t) => [t.name, colorValue(t.name, themed(t, theme))]),
  ...tokens.shadow.tokens.map((t) => [t.name, themed(t, theme)]),
])}
}`

function utility(style, group) {
  const family = style.family ?? group.family
  const lines = [
    `font-family: var(--font-${family});`,
    `font-size: ${style.fontSize};`,
    `line-height: ${style.lineHeight};`,
    `font-weight: ${style.fontWeight};`,
  ]
  if (style.letterSpacing) lines.push(`letter-spacing: ${style.letterSpacing};`)
  // README DS: admin-stat — «с табличными цифрами», в tokens.json это не выражено.
  if (style.name === 'admin-stat') lines.push('font-variant-numeric: tabular-nums;')
  return `@utility ${style.name} {\n${lines.map((l) => `  ${l}`).join('\n')}\n}`
}

export function generate(tokens) {
  const scalars = [...tokens.spacing.tokens, ...tokens.radius.tokens, ...tokens.size.tokens]
  const fonts = Object.entries(tokens.type.families).map(([k, v]) => [`font-${k}`, v])
  // Классы Tailwind: цвета, радиусы, тени. Отступы — через --spacing, размеры — h-(--size-cell).
  const theme = [
    ...tokens.color.tokens.map((t) => [`color-${t.name}`, `var(--${t.name})`]),
    ...tokens.radius.tokens.map((t) => [t.name, `var(--${t.name})`]),
    ...tokens.shadow.tokens.map((t) => [t.name, `var(--${t.name})`]),
  ]
  const utilities = tokens.type.groups.flatMap((g) => g.styles.map((s) => utility(s, g)))

  return `/* Сгенерировано scripts/gen-tokens.mjs из design/tokens.json. Не править руками: pnpm tokens. */
${themeBlock(':root,\n[data-theme="light"]', tokens, 'light')}

${themeBlock('[data-theme="dark"]', tokens, 'dark')}

:root {
${decls([...scalars.map((t) => [t.name, t.value]), ...fonts])}
}

/* Палитра и шкалы Tailwind по умолчанию выключены: в классах только токены. */
@theme inline {
  --color-*: initial;
  --radius-*: initial;
  --shadow-*: initial;
  --text-*: initial;
${decls(theme)}
}

${utilities.join('\n\n')}
`
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const read = (rel) => new URL(rel, import.meta.url)
  const tokens = JSON.parse(readFileSync(read('../design/tokens.json'), 'utf8'))
  writeFileSync(read('../src/styles/tokens.css'), generate(tokens))
}
