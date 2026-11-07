// Правки раскладки клавиатуры: ряды видимых кнопок (до 2 в ряду, блок один в ряду) и скрытые. Чистые функции;
// последнее слово за сервером (Keyboard.ValidateLayout), здесь только то, что нельзя собрать из кнопок интерфейса.
export const MAX_ROW = 2
export const MAX_ROWS = 8

export interface Layout {
  rows: string[][]
  hidden: string[]
}

export const sameLayout = (a: Layout, b: Layout) =>
  JSON.stringify(a.rows) === JSON.stringify(b.rows) && JSON.stringify([...a.hidden].sort()) === JSON.stringify([...b.hidden].sort())

/**
 * Куда бросили перетаскиваемую кнопку:
 * - cell: на другую ячейку; h/v — половина ячейки под указателем (слева/справа, сверху/снизу);
 * - beside: в пустое место рядом с одиночной ячейкой id (after — справа);
 * - gap: между рядами (index — перед рядом с этим номером, rows.length — в конец).
 */
export type Target =
  | { kind: 'cell'; id: string; h: 'left' | 'right'; v: 'top' | 'bottom' }
  | { kind: 'beside'; id: string; after: boolean }
  | { kind: 'gap'; index: number }

const without = (rows: string[][], id: string) => rows.map((r) => r.filter((x) => x !== id)).filter((r) => r.length > 0)

/**
 * Переносит кнопку id. В ряд кладёт, только если там одна обычная кнопка (до 2 в ряду, блок всегда один);
 * иначе — в новый ряд до/после. Нельзя (то же место, больше MAX_ROWS) — возвращает тот же l.
 */
function plan(l: Layout, id: string, target: Target, blocks: string[]): { layout: Layout; side?: 'left' | 'right' | 'top' | 'bottom' } {
  const from = l.rows.findIndex((r) => r.includes(id))
  if (from < 0) return { layout: l }
  const rows = without(l.rows, id)
  const emptied = l.rows[from].length === 1
  let next: string[][]
  let side: 'left' | 'right' | 'top' | 'bottom' | undefined
  if (target.kind === 'gap') {
    if (target.index < 0 || target.index > l.rows.length) return { layout: l }
    const at = target.index - (emptied && from < target.index ? 1 : 0)
    next = [...rows.slice(0, at), [id], ...rows.slice(at)]
  } else {
    if (target.id === id) return { layout: l }
    const tr = rows.findIndex((r) => r.includes(target.id))
    if (tr < 0) return { layout: l }
    const row = rows[tr]
    const joinable = row.length < MAX_ROW && !blocks.includes(id) && !row.some((x) => blocks.includes(x))
    if (target.kind === 'beside' && !joinable) return { layout: l }
    const after = target.kind === 'beside' ? target.after : joinable ? target.h === 'right' : target.v === 'bottom'
    if (joinable) {
      const at = row.indexOf(target.id) + (after ? 1 : 0)
      next = rows.map((r, i) => (i === tr ? [...r.slice(0, at), id, ...r.slice(at)] : r))
      side = after ? 'right' : 'left'
    } else {
      next = [...rows.slice(0, after ? tr + 1 : tr), [id], ...rows.slice(after ? tr + 1 : tr)]
      side = after ? 'bottom' : 'top'
    }
  }
  if (next.length > MAX_ROWS || JSON.stringify(next) === JSON.stringify(l.rows)) return { layout: l }
  return { layout: { ...l, rows: next }, side }
}

export const moveSlot = (l: Layout, id: string, target: Target, blocks: string[]): Layout => plan(l, id, target, blocks).layout

/** С какой стороны ячейки target встанет кнопка (для подсветки); undefined — бросить сюда нельзя. */
export const dropSide = (l: Layout, id: string, target: Target, blocks: string[]) => {
  const p = plan(l, id, target, blocks)
  return p.layout === l ? undefined : (p.side ?? 'gap')
}

export const hide = (l: Layout, id: string): Layout => ({
  rows: l.rows.map((row) => row.filter((x) => x !== id)).filter((row) => row.length > 0),
  hidden: [...l.hidden, id],
})

export const canShow = (l: Layout) => l.rows.length < MAX_ROWS

export const show = (l: Layout, id: string): Layout =>
  canShow(l) ? { rows: [...l.rows, [id]], hidden: l.hidden.filter((x) => x !== id) } : l
