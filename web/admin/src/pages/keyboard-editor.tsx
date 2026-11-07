import type { ReactNode } from 'react'
import { Button, Card, DragHandle, Icon, Input, LayoutGrid, StyleSelect } from '@/ui'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/ui/primitives/tooltip'
import type { KeyboardInfo, KeyboardSlot } from '../types'
import { canShow, hide, show, type Layout } from '../lib/keyboard-layout'

const BUTTON_LIMIT = 64

/** Что редактор знает о кнопке-слоте: тексты и стиль (черновик или сохранённое), id и действие из реестра. */
export interface SlotView {
  title: string
  label: string
  style: string
  route?: string
}

export interface ButtonCellProps {
  id: string
  view: SlotView
  busy: boolean
  onLabel(v: string): void
  onStyle(v: string): void
  /** Слева и справа в строке с id: ручка перетаскивания, «скрыть». */
  lead?: ReactNode
  trail?: ReactNode
}

/** Подпись и стиль одной кнопки; id и действие — строкой над полем. Общая для клавиатур и кнопок без раскладки. */
export function ButtonFields({ id, view, busy, onLabel, onStyle, lead, trail }: ButtonCellProps) {
  return (
    <>
      <div className="caption-1 flex min-w-0 items-center gap-1.5 font-mono text-text-secondary">
        {lead}
        <span className="flex min-w-0 grow flex-col">
          <code className="truncate">{id}</code>
          {view.route && <code className="truncate">→ {view.route}</code>}
        </span>
        {trail}
      </div>
      <div className="flex items-start gap-2">
        <Input
          className="min-w-0 grow"
          placeholder={view.title}
          aria-label={`Текст кнопки ${view.title}`}
          maxLength={BUTTON_LIMIT}
          disabled={busy}
          value={view.label}
          onChange={(e) => onLabel(e.target.value)}
        />
        <StyleSelect aria-label={`Стиль кнопки ${view.title}`} disabled={busy} value={view.style} onChange={onStyle} />
      </div>
    </>
  )
}

const cell = 'flex min-w-0 flex-col gap-1.5 rounded-md bg-fill-tertiary p-2'

interface KeyboardCardProps {
  kb: KeyboardInfo
  layout: Layout
  view(slot: KeyboardSlot): SlotView
  messageTitles: string[]
  busy: boolean
  dirty: boolean
  onLayout(l: Layout): void
  onLabel(id: string, v: string): void
  onStyle(id: string, v: string): void
  onSave(): void
  onCancel(): void
  onReset(): void
  saving: boolean
}

export function KeyboardCard({ kb, layout, view, messageTitles, busy, dirty, onLayout, onLabel, onStyle, onSave, onCancel, onReset, saving }: KeyboardCardProps) {
  const slot = (id: string) => kb.slots.find((s) => s.id === id)
  const blocks = kb.slots.filter((s) => s.kind === 'dynamic').map((s) => s.id)
  const name = (s: KeyboardSlot) => (s.kind === 'dynamic' ? s.label : view(s).title)

  return (
    <Card title={`Клавиатура: ${kb.title}`} className="gap-3">
      {messageTitles.length > 0 && (
        <details className="footnote text-text-secondary">
          <summary className="cursor-pointer">Общая для {messageTitles.length} сообщений</summary>
          <p className="pt-1">{messageTitles.join(', ')}</p>
        </details>
      )}
      {kb.inactive && (
        <p role="alert" className="admin-body rounded-lg bg-destructive-tint px-4 py-3 text-destructive-strong">
          Сохранённая раскладка не применяется, бот показывает расположение по умолчанию: {kb.problem}
        </p>
      )}
      <TooltipProvider delayDuration={200}>
        <LayoutGrid
          layout={layout}
          blocks={blocks}
          disabled={busy}
          name={(id) => {
            const s = slot(id)
            return s ? name(s) : id
          }}
          onChange={onLayout}
          renderCell={(id) => {
            const s = slot(id)
            if (!s) return null
            const n = name(s)
            const trail =
              s.kind !== 'button' ? null : s.removable ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      aria-label={`Скрыть: ${n}`}
                      disabled={busy}
                      onClick={() => onLayout(hide(layout, id))}
                      className="focus-ring inline-flex size-6 flex-none cursor-pointer items-center justify-center rounded-sm text-text-secondary hover:bg-cell-highlight disabled:cursor-default disabled:opacity-40"
                    >
                      <Icon name="close" size={14} strokeWidth={2.4} />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>Скрыть</TooltipContent>
                </Tooltip>
              ) : (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span tabIndex={0} role="img" aria-label={`Нужна для работы бота: ${n}`} className="focus-ring inline-flex size-6 items-center justify-center rounded-full text-text-secondary">
                      <Icon name="lock" size={14} />
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>Нужна для работы бота</TooltipContent>
                </Tooltip>
              )
            return s.kind === 'dynamic' ? (
              <div className={`${cell} border border-dashed border-separator bg-transparent`}>
                <div className="flex items-center gap-1.5">
                  <DragHandle />
                  <code className="caption-1 truncate font-mono text-text-secondary">{id}</code>
                </div>
                <span className="admin-body text-text-secondary">{s.label}</span>
              </div>
            ) : (
              <div className={cell}>
                <ButtonFields id={id} view={view(s)} busy={busy} onLabel={(v) => onLabel(id, v)} onStyle={(v) => onStyle(id, v)} lead=<DragHandle /> trail={trail} />
              </div>
            )
          }}
        />
      </TooltipProvider>

      {layout.hidden.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="admin-label text-text-secondary">Скрытые кнопки</span>
          <ul className="flex flex-wrap gap-1.5">
            {layout.hidden.map((id) => {
              const s = slot(id)
              if (!s) return null
              return (
                <li key={id} className="flex items-center gap-1 rounded-full bg-fill-tertiary py-0.5 pr-1 pl-3">
                  <span className="admin-body">{name(s)}</span>
                  <Button variant="plain" size="sm" disabled={busy || !canShow(layout)} aria-label={`Показать: ${name(s)}`} onClick={() => onLayout(show(layout, id))}>
                    Показать
                  </Button>
                </li>
              )
            })}
          </ul>
        </div>
      )}

      <p className="footnote text-text-secondary">
        Что за кнопка и когда она появляется, решает код бота; здесь — подпись, стиль, порядок (перетащите ручку: на соседнюю кнопку — в один ряд, между рядами — в новый) и «за ненадобностью» скрытие. Подпись и стиль общие для всех
        экранов с этой кнопкой.
      </p>
      <div className="flex flex-wrap justify-end gap-3">
        {dirty && (
          <Button variant="plain" disabled={busy} onClick={onCancel}>
            Отменить
          </Button>
        )}
        <Button variant="gray" icon="refresh" disabled={busy || !kb.overridden} onClick={onReset}>
          Вернуть как в коде
        </Button>
        <Button loading={saving} disabled={busy || !dirty} onClick={onSave}>
          Сохранить клавиатуру
        </Button>
      </div>
    </Card>
  )
}
