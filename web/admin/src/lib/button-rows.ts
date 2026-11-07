import type { Layout } from './keyboard-layout'

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

// Кнопки рассылки ↔ раскладка для перетаскивания: id ячейки — индекс кнопки в списке.
export const toLayout = (items: { inline?: boolean }[]): Layout => ({ rows: buttonRows(items).map((r) => r.map((x) => String(x.index))), hidden: [] })

export function fromLayout<T extends { inline?: boolean }>(items: T[], l: Layout): T[] {
  return l.rows.flatMap((row) =>
    row.map((id, i) => {
      const b = { ...items[Number(id)] }
      if (i > 0) b.inline = true
      else delete b.inline
      return b
    }),
  )
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
