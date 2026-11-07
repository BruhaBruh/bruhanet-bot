// Клавиатура сообщения для редактора: чистые функции над рядами ячеек (правила — на сервере, texts.ValidateMessageLayout).
import type { KbAction, KbCell, KbCellOut, KbTemplate } from '../types'
import type { Layout } from './keyboard-layout'

export const MAX_CELLS = 16

export const toLayout = (rows: KbCellOut[][]): Layout => ({ rows: rows.map((r) => r.map((c) => c.id)), hidden: [] })

/** Ряды ячеек в порядке раскладки (после перетаскивания); неизвестные id отбрасываются. */
export function fromLayout(rows: KbCellOut[][], l: Layout): KbCellOut[][] {
  const byId = new Map(rows.flat().map((c) => [c.id, c]))
  return l.rows.map((r) => r.flatMap((id) => byId.get(id) ?? []))
}

/** Только то, что принимает PUT: без вычисляемых полей (locked, template, effective). */
export const strip = (c: KbCellOut): KbCell => ({
  id: c.id,
  ...(c.key ? { key: c.key } : {}),
  ...(c.label ? { label: c.label } : {}),
  ...(c.style ? { style: c.style } : {}),
  action: c.action,
  conditions: c.conditions.map((x) => (x.not ? { id: x.id, not: true } : { id: x.id })),
})

export const payload = (rows: KbCellOut[][]) => ({ rows: rows.map((r) => r.map(strip)) })

export const sameKb = (a: KbCellOut[][], b: KbCellOut[][]) => JSON.stringify(payload(a)) === JSON.stringify(payload(b))

export const patchCell = (rows: KbCellOut[][], id: string, fn: (c: KbCellOut) => KbCellOut) => rows.map((r) => r.map((c) => (c.id === id ? fn(c) : c)))

export const removeCell = (rows: KbCellOut[][], id: string) => rows.map((r) => r.filter((c) => c.id !== id)).filter((r) => r.length > 0)

export const countCells = (rows: KbCellOut[][]) => rows.reduce((n, r) => n + r.length, 0)

const taken = (rows: KbCellOut[][]) => new Set(rows.flat().map((c) => c.id))

/** Свободный id для новой кнопки: `btn_1`, `btn_2`… */
export function freeId(rows: KbCellOut[][], prefix = 'btn'): string {
  const used = taken(rows)
  for (let i = 1; ; i++) if (!used.has(`${prefix}_${i}`)) return `${prefix}_${i}`
}

/** Новая своя кнопка в новом ряду: ссылка из меню — безопасное действие, которое проходит любую проверку. */
export const addButton = (rows: KbCellOut[][]): KbCellOut[][] => [
  ...rows,
  [{ id: freeId(rows), label: 'Кнопка', action: 'route:menu', conditions: [], locked: false, template: false, effective: { label: 'Кнопка', preview: 'Кнопка', style: '' } }],
]

/** Шаблон встаёт отдельным рядом; id шаблона = id его действия (`template:<id>`). */
export const addTemplate = (rows: KbCellOut[][], t: KbTemplate): KbCellOut[][] => [
  ...rows,
  [{ id: `@${t.id}`, action: `template:${t.id}`, conditions: [], locked: false, template: true }],
]

export const templatesLeft = (rows: KbCellOut[][], all: KbTemplate[]) => {
  const used = taken(rows)
  return all.filter((t) => !used.has(`@${t.id}`) && !rows.flat().some((c) => c.action === `template:${t.id}`))
}

/** Кнопка видна при фейк-условиях `on`: все условия выполнены (с учётом НЕ) и встроенное условие действия тоже. */
export function visibleRows(rows: KbCellOut[][], on: ReadonlySet<string>, actions: KbAction[]): KbCellOut[][] {
  const guard = new Map(actions.map((a) => [a.id, a.guard]))
  const ok = (c: KbCellOut) => {
    const g = guard.get(c.action)
    return (!g || on.has(g)) && c.conditions.every((x) => on.has(x.id) !== !!x.not)
  }
  return rows.map((r) => r.filter(ok)).filter((r) => r.length > 0)
}

/** Условия, о которых знает клавиатура (свои и встроенные в действия): их и показываем чекбоксами. */
export function usedConditions(rows: KbCellOut[][], actions: KbAction[]): string[] {
  const guard = new Map(actions.map((a) => [a.id, a.guard]))
  const ids = new Set<string>()
  for (const c of rows.flat()) {
    c.conditions.forEach((x) => ids.add(x.id))
    const g = guard.get(c.action)
    if (g) ids.add(g)
  }
  return [...ids]
}
