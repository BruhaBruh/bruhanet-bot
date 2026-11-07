import '@xyflow/react/dist/style.css'
import { Background, Controls, Handle, MarkerType, Position, ReactFlow, type Edge, type Node, type NodeProps } from '@xyflow/react'
import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { Link } from 'react-router'
import { cn } from '@/lib/utils'
import { Checkbox, MessageBubble, Spinner } from '@/ui'
import { get } from '../api'
import { previewHtml, renderPlain, renderPreview } from '../editor/serialize'
import { previewStyle } from '../lib/button-rows'
import { usedConditions } from '../lib/message-keyboard'
import { buildGraph, layout, type EntryNodeData, type ScreenNodeData, type UxMapTables } from '../lib/uxmap'
import type { TextsReply } from '../types'

interface ScreenData extends ScreenNodeData {
  html: string
  samples: Record<string, string>
  [key: string]: unknown
}

const handleDot = '!size-2 !border-0 !bg-text-secondary'

function ScreenNode({ data }: NodeProps<Node<ScreenData>>) {
  const { item, rows, html, samples } = data
  return (
    <div className="w-75 rounded-xl bg-chat-bg p-2 shadow-card">
      <Handle type="target" position={Position.Left} className={handleDot} />
      <Handle type="source" position={Position.Right} className={handleDot} />
      <div className="flex items-start justify-between gap-2 px-1 pb-1.5">
        <div className="flex min-w-0 flex-col">
          <span className="admin-label truncate text-text">{item.title}</span>
          <code className="caption-1 truncate font-mono text-text-secondary">{item.key}</code>
        </div>
        <Link to={`/texts?key=${encodeURIComponent(item.key)}`} className="nodrag nopan footnote focus-ring flex-none rounded-sm bg-fill-tertiary px-2 py-1 text-accent-strong">
          Править
        </Link>
      </div>
      <MessageBubble
        time="14:30"
        keyboard={rows.map((row) =>
          row.map((b) => ({
            text: b.template ? `Шаблон: ${b.cell.action.replace('template:', '')}` : (b.cell.label ? renderPlain(b.cell.label, samples) : b.cell.effective?.preview) || b.id,
            style: previewStyle(b.template ? '' : b.cell.style || b.cell.effective?.style || ''),
            handle: <Handle type="source" position={Position.Right} id={b.id} className={cn(handleDot, '!-right-3')} />,
          })),
        )}
      >
        <span className="whitespace-pre-wrap" dangerouslySetInnerHTML={{ __html: html }} />
      </MessageBubble>
    </div>
  )
}

const kindTone = { command: 'bg-accent-tint text-accent-strong', event: 'bg-warning-tint text-warning-strong' }

function EntryNode({ data }: NodeProps<Node<EntryNodeData & { [key: string]: unknown }>>) {
  const { entry } = data
  return (
    <div className={cn('w-55 rounded-xl px-3 py-2 shadow-card', kindTone[entry.kind])}>
      <Handle type="source" position={Position.Right} className={handleDot} />
      <div className="admin-label">{entry.title}</div>
      <div className="footnote opacity-80">{entry.when}</div>
    </div>
  )
}

const nodeTypes = { screen: ScreenNode, entry: EntryNode }
const ARROW = '#8e8e93'
// Цвета xyflow из токенов админки: подсветка и кнопки управления следуют теме.
const flowTheme = {
  '--xy-controls-button-background-color': 'var(--surface)',
  '--xy-controls-button-background-color-hover': 'var(--cell-highlight)',
  '--xy-controls-button-color': 'var(--text)',
  '--xy-controls-button-border-color': 'var(--separator)',
  '--xy-background-color': 'var(--page-bg)',
} as CSSProperties

export default function UxMap() {
  const [texts, setTexts] = useState<TextsReply>()
  const [map, setMap] = useState<UxMapTables>()
  const [failed, setFailed] = useState(false)
  const [on, setOn] = useState<string[]>([])
  const [groups, setGroups] = useState<string[]>([])

  useEffect(() => {
    Promise.all([get<TextsReply>('/api/texts'), get<UxMapTables>('/api/uxmap')]).then(
      ([t, m]) => {
        setTexts(t)
        setMap(m)
      },
      () => setFailed(true),
    )
  }, [])

  const actions = useMemo(() => texts?.actions ?? [], [texts])
  const allGroups = useMemo(() => [...new Set((texts?.items ?? []).filter((i) => i.kind === 'message').map((i) => i.group))], [texts])
  // условия, которые вообще встречаются в клавиатурах: остальные чекбоксы ничего бы не меняли
  const conditionIds = useMemo(() => {
    const rows = (texts?.items ?? []).flatMap((i) => i.keyboard?.rows ?? [])
    return new Set(usedConditions(rows, actions))
  }, [texts, actions])

  const graph = useMemo(() => {
    if (!texts || !map) return undefined
    const g = buildGraph(texts.items, map, { on: new Set(on), actions, groups: new Set(groups) })
    const positioned = layout(g.nodes, g.edges)
    const nodes: Node[] = positioned.map((n) => {
      if (n.type === 'entry') return { id: n.id, type: 'entry', position: { x: n.x, y: n.y }, data: n.data as unknown as Record<string, unknown> }
      const d = n.data as ScreenNodeData
      const vars = [...texts.globals, ...d.item.vars]
      const samples = Object.fromEntries(vars.map((v) => [v.name, v.sample]))
      const raw = vars.filter((v) => v.raw).map((v) => v.name)
      return {
        id: n.id,
        type: 'screen',
        position: { x: n.x, y: n.y },
        data: { ...d, samples, html: previewHtml(renderPreview(d.item.body, samples, raw)) } as ScreenData,
      }
    })
    const edges: Edge[] = g.edges.map((e) => ({
      id: e.id,
      source: e.source,
      sourceHandle: e.sourceHandle,
      target: e.target,
      label: e.label,
      type: 'smoothstep',
      style: { stroke: ARROW, strokeWidth: 1.5, strokeDasharray: e.kind === 'button' ? undefined : '6 4' },
      markerEnd: { type: MarkerType.ArrowClosed, color: ARROW },
      labelStyle: { fill: 'var(--text)', fontSize: 11 },
      labelBgStyle: { fill: 'var(--surface)' },
      labelBgPadding: [4, 2] as [number, number],
    }))
    return { nodes, edges }
  }, [texts, map, on, groups, actions])

  if (failed)
    return (
      <p role="alert" className="admin-body text-destructive-strong">
        Не удалось загрузить
      </p>
    )
  if (!texts || !graph) return <Spinner className="self-center p-8" />

  const toggle = (list: string[], set: (v: string[]) => void, v: string) => set(list.includes(v) ? list.filter((x) => x !== v) : [...list, v])

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 rounded-lg bg-surface px-4 py-3 shadow-card">
        <span className="admin-label text-text-secondary">Условия для превью</span>
        {(texts.conditions ?? [])
          .filter((c) => conditionIds.has(c.id))
          .map((c) => (
            <Checkbox key={c.id} checked={on.includes(c.id)} label={c.title} onChange={() => toggle(on, setOn, c.id)} />
          ))}
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="admin-label px-1 text-text-secondary">Группы</span>
        {allGroups.map((g) => (
          <button
            key={g}
            type="button"
            aria-pressed={groups.includes(g)}
            className={cn('admin-label focus-ring cursor-pointer rounded-md px-2.5 py-1.5', groups.includes(g) ? 'bg-accent-strong text-on-accent-strong' : 'bg-surface text-text')}
            onClick={() => toggle(groups, setGroups, g)}
          >
            {g}
          </button>
        ))}
        <span className="footnote px-1 text-text-secondary">{groups.length ? 'показаны выбранные' : 'показаны все'}</span>
      </div>
      <div className="h-[75dvh] min-h-120 overflow-hidden rounded-lg bg-page-bg shadow-card" aria-label="Схема бота">
        <ReactFlow
          key={groups.join('|')}
          nodes={graph.nodes}
          edges={graph.edges}
          nodeTypes={nodeTypes}
          fitView
          minZoom={0.05}
          maxZoom={1.5}
          nodesConnectable={false}
          proOptions={{ hideAttribution: true }}
          style={flowTheme}
        >
          <Background />
          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
      <p className="footnote text-text-secondary">
        Сплошная стрелка — кнопка, пунктир — переход без кнопки (ввод юзера, решение админа, событие). Схема показывает клавиатуры такими, как их применяет бот.
      </p>
    </div>
  )
}
