// Правки раскладки клавиатуры: ряды видимых кнопок (до 2 в ряду, блок один в ряду) и скрытые. Чистые функции;
// последнее слово за сервером (Keyboard.ValidateLayout), здесь только то, что нельзя собрать из кнопок интерфейса.
export const MAX_ROW = 2
export const MAX_ROWS = 8

export interface Layout {
  rows: string[][]
  hidden: string[]
}

const swap = <T>(a: T[], i: number, j: number) => {
  const r = [...a]
  ;[r[i], r[j]] = [r[j], r[i]]
  return r
}

export const sameLayout = (a: Layout, b: Layout) =>
  JSON.stringify(a.rows) === JSON.stringify(b.rows) && JSON.stringify([...a.hidden].sort()) === JSON.stringify([...b.hidden].sort())

export const moveRow = (l: Layout, r: number, dir: -1 | 1): Layout => {
  const to = r + dir
  return to < 0 || to >= l.rows.length ? l : { ...l, rows: swap(l.rows, r, to) }
}

export const moveInRow = (l: Layout, r: number, c: number, dir: -1 | 1): Layout => {
  const to = c + dir
  if (to < 0 || to >= l.rows[r].length) return l
  return { ...l, rows: l.rows.map((row, i) => (i === r ? swap(row, c, to) : row)) }
}

// Первая в ряду уходит в ряд выше (если там место и нет блока), остальные — в свой ряд ниже.
export function canJoin(l: Layout, r: number, c: number, blocks: string[]): boolean {
  const id = l.rows[r][c]
  if (c > 0 || r === 0 || blocks.includes(id)) return false
  const prev = l.rows[r - 1]
  return prev.length < MAX_ROW && !prev.some((x) => blocks.includes(x))
}

export const canSplit = (l: Layout, r: number, c: number): boolean => c > 0 && l.rows.length < MAX_ROWS

export function toggleJoin(l: Layout, r: number, c: number, blocks: string[]): Layout {
  const id = l.rows[r][c]
  if (c > 0) {
    if (!canSplit(l, r, c)) return l
    const rows = l.rows.map((row, i) => (i === r ? row.filter((x) => x !== id) : row))
    rows.splice(r + 1, 0, [id])
    return { ...l, rows }
  }
  if (!canJoin(l, r, c, blocks)) return l
  const rows = l.rows.map((row, i) => (i === r - 1 ? [...row, id] : i === r ? row.slice(1) : row)).filter((row) => row.length > 0)
  return { ...l, rows }
}

export const hide = (l: Layout, id: string): Layout => ({
  rows: l.rows.map((row) => row.filter((x) => x !== id)).filter((row) => row.length > 0),
  hidden: [...l.hidden, id],
})

export const canShow = (l: Layout) => l.rows.length < MAX_ROWS

export const show = (l: Layout, id: string): Layout =>
  canShow(l) ? { rows: [...l.rows, [id]], hidden: l.hidden.filter((x) => x !== id) } : l
