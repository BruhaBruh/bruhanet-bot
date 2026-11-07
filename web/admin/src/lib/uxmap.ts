// UX-карта бота: из сообщений с клавиатурами (GET /api/texts) и таблиц переходов (GET /api/uxmap) строим граф.
// Чистые функции: рендер и раскладка на экране — в pages/UxMap.tsx.
import dagre from '@dagrejs/dagre'
import type { KbAction, KbCellOut, TextItem } from '../types'
import { visibleRows } from './message-keyboard'

export interface MapEntry {
  id: string
  kind: 'command' | 'event'
  title: string
  when: string
  targets: { to: string; label?: string }[]
}
export interface MapTransition {
  from: string
  action: string
  to: string[]
}
export interface MapLink {
  from: string
  to: string
  label: string
}
export interface UxMapTables {
  entries: MapEntry[]
  transitions: MapTransition[]
  links: MapLink[]
}

/** Куда ведёт действие кнопки на экране from: запись экрана главнее общей («*»). */
export function targetsOf(map: UxMapTables, from: string, action: string): string[] {
  const own = map.transitions.find((t) => t.from === from && t.action === action)
  return (own ?? map.transitions.find((t) => t.from === '*' && t.action === action))?.to ?? []
}

export interface ScreenButton {
  id: string
  cell: KbCellOut
  template: boolean
}
export interface ScreenNodeData {
  item: TextItem
  /** Кнопки видимых клавиатурных рядов (шаблон — одна кнопка-блок). */
  rows: ScreenButton[][]
}
export interface EntryNodeData {
  entry: MapEntry
}
export interface GraphNode {
  id: string
  type: 'screen' | 'entry'
  data: ScreenNodeData | EntryNodeData
  width: number
  height: number
  x: number
  y: number
}
export interface GraphEdge {
  id: string
  source: string
  sourceHandle?: string
  target: string
  label?: string
  kind: 'button' | 'link' | 'entry'
}

export const SCREEN_WIDTH = 300
const ENTRY_WIDTH = 220
const ENTRY_HEIGHT = 64

/** Высота узла до измерения: достаточно для раскладки, точные размеры рисует браузер. */
export function screenHeight(body: string, rows: number): number {
  const text = body.replace(/<[^>]*>/g, '')
  const lines = text.split('\n').reduce((n, l) => n + Math.max(1, Math.ceil(l.length / 34)), 0)
  return 56 + lines * 18 + 24 + rows * 44
}

export interface BuildOptions {
  /** Выполненные «условия для превью»: их проходят кнопки, у остальных кнопок нет и стрелок тоже. */
  on: ReadonlySet<string>
  actions: KbAction[]
  /** Показанные группы сообщений (Меню, Оплата…); пусто — все. */
  groups?: ReadonlySet<string>
}

/** Узлы (экраны и входы) и рёбра карты; координаты — после layout. */
export function buildGraph(items: TextItem[], map: UxMapTables, o: BuildOptions): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const screens = items.filter((i) => i.kind === 'message' && (!o.groups?.size || o.groups.has(i.group)))
  const shown = new Set(screens.map((i) => i.key))
  const nodes: GraphNode[] = []
  const edges: GraphEdge[] = []
  const edge = (e: GraphEdge) => {
    if (shown.has(e.target) && (e.source.startsWith('entry:') || shown.has(e.source))) edges.push(e)
  }

  for (const item of screens) {
    const rows = visibleRows(item.keyboard?.rows ?? [], o.on, o.actions).map((r) => r.map((cell) => ({ id: cell.id, cell, template: cell.template })))
    nodes.push({ id: item.key, type: 'screen', data: { item, rows }, width: SCREEN_WIDTH, height: screenHeight(item.body, rows.length), x: 0, y: 0 })
    for (const btn of rows.flat()) {
      for (const to of targetsOf(map, item.key, btn.cell.action)) {
        edge({ id: `${item.key}:${btn.id}->${to}`, source: item.key, sourceHandle: btn.id, target: to, kind: 'button' })
      }
    }
  }
  for (const l of map.links) edge({ id: `link:${l.from}->${l.to}`, source: l.from, target: l.to, label: l.label, kind: 'link' })

  for (const entry of map.entries) {
    if (!entry.targets.some((t) => shown.has(t.to))) continue
    const id = `entry:${entry.id}`
    nodes.push({ id, type: 'entry', data: { entry }, width: ENTRY_WIDTH, height: ENTRY_HEIGHT, x: 0, y: 0 })
    for (const t of entry.targets) edge({ id: `${id}->${t.to}`, source: id, target: t.to, label: t.label, kind: 'entry' })
  }
  return { nodes, edges }
}

/** Слева направо: входы → экраны. Меняет x и y узлов (центр узла переводится в левый верхний угол). */
export function layout(nodes: GraphNode[], edges: GraphEdge[]): GraphNode[] {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 40, ranksep: 120, marginx: 24, marginy: 24 })
  g.setDefaultEdgeLabel(() => ({}))
  for (const n of nodes) g.setNode(n.id, { width: n.width, height: n.height })
  for (const e of edges) g.setEdge(e.source, e.target)
  dagre.layout(g)
  return nodes.map((n) => {
    const p = g.node(n.id)
    return { ...n, x: p.x - n.width / 2, y: p.y - n.height / 2 }
  })
}
