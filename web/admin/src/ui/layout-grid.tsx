/* eslint-disable react-hooks/refs -- @dnd-kit отдаёт ref-колбэки и атрибуты хуком useDraggable; правило принимает их за чтение ref.current */
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  pointerWithin,
  rectIntersection,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type Announcements,
  type CollisionDetection,
  type DragEndEvent,
  type DragMoveEvent,
} from '@dnd-kit/core'
import { Fragment, createContext, useContext, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { dropSide, moveSlot, type Layout, type Target } from '../lib/keyboard-layout'
import { Icon } from './icon'

export interface LayoutGridProps {
  layout: Layout
  /** Блоки — всегда одни в ряду. */
  blocks: string[]
  disabled?: boolean
  /** Подпись кнопки: для диктора, ручки и «шлейфа» при перетаскивании. */
  name(id: string): string
  onChange(l: Layout): void
  /** Содержимое ячейки; ручку перетаскивания (`<DragHandle />`) надо поставить внутрь. */
  renderCell(id: string): ReactNode
}

const collide: CollisionDetection = (args) => {
  const hit = pointerWithin(args)
  return hit.length ? hit : rectIntersection(args)
}

const BAR = 'pointer-events-none absolute z-10 rounded-full bg-accent'

/** Сетка рядов (до 2 ячеек) с перетаскиванием: мышь, касание и клавиатура (@dnd-kit). Правила — lib/keyboard-layout. */
export function LayoutGrid({ layout, blocks, disabled, name, onChange, renderCell }: LayoutGridProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 150, tolerance: 6 } }),
    useSensor(KeyboardSensor),
  )
  const [active, setActive] = useState<string>()
  const [target, setTarget] = useState<Target>()
  const side = active && target ? dropSide(layout, active, target, blocks) : undefined

  function resolve(e: DragMoveEvent | DragEndEvent): Target | undefined {
    if (!e.over) return undefined
    const [kind, ...rest] = String(e.over.id).split(':')
    const id = rest.join(':')
    if (kind === 'gap') return { kind: 'gap', index: Number(id) }
    if (kind === 'beside') return { kind: 'beside', id, after: true }
    const ae = e.activatorEvent as Partial<PointerEvent>
    const moved = e.active.rect.current.translated
    const px = typeof ae.clientX === 'number' ? ae.clientX + e.delta.x : moved ? moved.left + moved.width / 2 : 0
    const py = typeof ae.clientY === 'number' ? ae.clientY + e.delta.y : moved ? moved.top + moved.height / 2 : 0
    const r = e.over.rect
    return { kind: 'cell', id, h: px < r.left + r.width / 2 ? 'left' : 'right', v: py < r.top + r.height / 2 ? 'top' : 'bottom' }
  }

  const reset = () => {
    setActive(undefined)
    setTarget(undefined)
  }
  const where = (id: unknown) => {
    const [kind, ...rest] = String(id).split(':')
    return kind === 'gap' ? 'между рядами' : `рядом с кнопкой «${name(rest.join(':'))}»`
  }
  const announcements: Announcements = {
    onDragStart: ({ active: a }) => `Взята кнопка «${name(String(a.id))}»`,
    onDragOver: ({ active: a, over }) => (over ? `Кнопка «${name(String(a.id))}» ${where(over.id)}` : `Кнопка «${name(String(a.id))}» вне сетки`),
    onDragEnd: ({ active: a, over }) => (over ? `Кнопка «${name(String(a.id))}» перемещена: ${where(over.id)}` : `Кнопка «${name(String(a.id))}» осталась на месте`),
    onDragCancel: ({ active: a }) => `Перемещение кнопки «${name(String(a.id))}» отменено`,
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collide}
      accessibility={{
        announcements,
        screenReaderInstructions: {
          draggable: 'Чтобы переместить кнопку, нажмите пробел, выберите место стрелками и снова нажмите пробел. Escape — отмена.',
        },
      }}
      onDragStart={(e) => setActive(String(e.active.id))}
      onDragMove={(e) => setTarget(resolve(e))}
      onDragOver={(e) => setTarget(resolve(e as DragMoveEvent))}
      onDragCancel={reset}
      onDragEnd={(e) => {
        const t = resolve(e)
        const id = String(e.active.id)
        reset()
        if (t) onChange(moveSlot(layout, id, t, blocks))
      }}
    >
      <div className="flex flex-col">
        {layout.rows.map((row, r) => (
          <Fragment key={row[0]}>
            <Gap index={r} dragging={!!active} lit={side === 'gap' && target?.kind === 'gap' && target.index === r} />
            <div className="grid grid-cols-2 gap-2">
              {row.map((id) => (
                <Cell
                  key={id}
                  id={id}
                  label={name(id)}
                  block={blocks.includes(id)}
                  disabled={disabled}
                  side={target?.kind === 'cell' && target.id === id ? side : undefined}
                  render={renderCell}
                />
              ))}
              {row.length === 1 && !blocks.includes(row[0]) && (
                <Ghost id={row[0]} dragging={!!active} lit={side === 'right' && target?.kind === 'beside' && target.id === row[0]} />
              )}
            </div>
          </Fragment>
        ))}
        <Gap index={layout.rows.length} trailing dragging={!!active} lit={side === 'gap' && target?.kind === 'gap' && target.index === layout.rows.length} />
      </div>
      <DragOverlay dropAnimation={null}>{active && <div className="admin-body rounded-md bg-surface px-3 py-2 shadow-popover">{name(active)}</div>}</DragOverlay>
    </DndContext>
  )
}

function Gap({ index, trailing, dragging, lit }: { index: number; trailing?: boolean; dragging: boolean; lit: boolean }) {
  const { setNodeRef } = useDroppable({ id: `gap:${index}` })
  return (
    <div ref={setNodeRef} className={cn('relative flex items-center justify-center', trailing ? 'h-6' : 'h-3')}>
      {trailing && dragging && <span className="footnote text-text-hint">Новый ряд</span>}
      {lit && <span className={cn(BAR, 'inset-x-0 h-1')} />}
    </div>
  )
}

function Ghost({ id, dragging, lit }: { id: string; dragging: boolean; lit: boolean }) {
  const { setNodeRef } = useDroppable({ id: `beside:${id}` })
  return <div ref={setNodeRef} aria-hidden className={cn('rounded-md border border-dashed', dragging ? 'border-separator' : 'border-transparent', lit && 'border-accent bg-accent-tint')} />
}

type HandleProps = Pick<ReturnType<typeof useDraggable>, 'listeners' | 'attributes' | 'setActivatorNodeRef'> & { label: string; disabled?: boolean }
const HandleContext = createContext<HandleProps | null>(null)

/** Ручка перетаскивания ячейки (grip); работает только внутри renderCell. */
export function DragHandle() {
  const h = useContext(HandleContext)
  if (!h) return null
  return (
    <button
      type="button"
      ref={h.setActivatorNodeRef}
      aria-label={`Перетащить: ${h.label}`}
      title="Перетащить"
      disabled={h.disabled}
      {...h.listeners}
      {...h.attributes}
      className="focus-ring inline-flex size-6 flex-none cursor-grab touch-none items-center justify-center rounded-sm text-text-secondary hover:bg-cell-highlight disabled:cursor-default disabled:opacity-40"
    >
      <Icon name="grip" size={16} strokeWidth={3} />
    </button>
  )
}

function Cell({ id, label, block, disabled, side, render }: { id: string; label: string; block: boolean; disabled?: boolean; side?: string; render: LayoutGridProps['renderCell'] }) {
  const drag = useDraggable({ id, disabled, attributes: { roleDescription: 'перетаскиваемая кнопка' } })
  const drop = useDroppable({ id: `cell:${id}` })
  const { listeners, attributes, setActivatorNodeRef, setNodeRef, isDragging } = drag
  return (
    <HandleContext.Provider value={{ listeners, attributes, setActivatorNodeRef, label, disabled }}>
      <div
        ref={(n) => {
          setNodeRef(n)
          drop.setNodeRef(n)
        }}
        className={cn('relative min-w-0', block && 'col-span-2', isDragging && 'opacity-40')}
      >
        {render(id)}
        {side === 'left' && <span className={cn(BAR, '-left-1.5 inset-y-0 w-1')} />}
        {side === 'right' && <span className={cn(BAR, '-right-1.5 inset-y-0 w-1')} />}
        {side === 'top' && <span className={cn(BAR, '-top-1.5 inset-x-0 h-1')} />}
        {side === 'bottom' && <span className={cn(BAR, '-bottom-1.5 inset-x-0 h-1')} />}
      </div>
    </HandleContext.Provider>
  )
}
