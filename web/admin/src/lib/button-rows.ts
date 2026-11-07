// Кнопки рассылки → строки: inline — в строку предыдущей (как core/broadcast.Rows), иначе новая строка.
export function buttonRows<T extends { inline?: boolean }>(items: T[]): { item: T; index: number }[][] {
  const rows: { item: T; index: number }[][] = []
  items.forEach((item, index) => {
    const last = rows[rows.length - 1]
    if (item.inline && last && last.length < 2) last.push({ item, index })
    else rows.push([{ item, index }])
  })
  return rows
}

export const previewStyle = (s: string) => (s || 'default') as 'default' | 'primary' | 'success' | 'danger'

// Можно ли включить «в строке с предыдущей»: не первая, и в строке предыдущей ещё нет двух (выключить можно всегда).
export function canInline(items: { inline?: boolean }[], i: number): boolean {
  if (i === 0) return false
  if (items[i].inline) return true
  const prev = buttonRows(items.slice(0, i)).at(-1)
  return !!prev && prev.length < 2
}

// Инварианты сервера: первая кнопка не inline, в строке не больше двух; false не храним.
export function normalizeInline<T extends { inline?: boolean }>(items: T[]): T[] {
  let inRow = 0
  return items.map((b, i) => {
    const inline = !!b.inline && i > 0 && inRow < 2
    inRow = inline ? inRow + 1 : 1
    if (inline === !!b.inline && (inline || b.inline === undefined)) return b
    const rest = { ...b }
    delete rest.inline
    return (inline ? { ...rest, inline } : rest) as T
  })
}
