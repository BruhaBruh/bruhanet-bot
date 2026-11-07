import { Button, Card, Icon, Input, StyleSelect, type IconName } from '@/ui'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/ui/primitives/tooltip'
import type { KeyboardInfo, KeyboardSlot } from '../types'
import { canJoin, canShow, canSplit, hide, moveInRow, moveRow, show, toggleJoin, type Layout } from './keyboard-layout'

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
}

/** Подпись и стиль одной кнопки; id и действие — строкой над полем. Общая для клавиатур и кнопок без раскладки. */
export function ButtonFields({ id, view, busy, onLabel, onStyle }: ButtonCellProps) {
  return (
    <>
      <div className="caption-1 flex min-w-0 flex-wrap gap-x-2 font-mono text-text-secondary">
        <code className="truncate">{id}</code>
        {view.route && <code className="truncate">→ {view.route}</code>}
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
  const { rows } = layout

  // Иконка-кнопка в одну строку управления; подпись — в подсказке и aria-label.
  const tool = (label: string, icon: IconName, disabled: boolean, onClick: () => void) => (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          disabled={busy || disabled}
          onClick={onClick}
          className="focus-ring inline-flex size-7 flex-none cursor-pointer items-center justify-center rounded-sm bg-surface text-text hover:bg-cell-highlight disabled:cursor-default disabled:opacity-40"
        >
          <Icon name={icon} size={16} strokeWidth={2} />
        </button>
      </TooltipTrigger>
      <TooltipContent>{label.split(': ')[0]}</TooltipContent>
    </Tooltip>
  )

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
        {rows.map((row, r) => (
          <div key={row[0]} className="grid gap-3 md:grid-cols-2">
            {row.map((id, c) => {
              const s = slot(id)
              if (!s) return null
              const n = name(s)
              return (
                <div key={id} className={s.kind === 'dynamic' ? `${cell} border border-dashed border-separator bg-transparent` : cell}>
                  {s.kind === 'dynamic' ? (
                    <>
                      <code className="caption-1 truncate font-mono text-text-secondary">{id}</code>
                      <span className="admin-body text-text-secondary">{s.label}</span>
                    </>
                  ) : (
                    <ButtonFields id={id} view={view(s)} busy={busy} onLabel={(v) => onLabel(id, v)} onStyle={(v) => onStyle(id, v)} />
                  )}
                  <div className="flex flex-wrap items-center gap-1">
                    {tool(`Выше: ${n}`, 'arrow-up', r === 0, () => onLayout(moveRow(layout, r, -1)))}
                    {tool(`Ниже: ${n}`, 'arrow-down', r === rows.length - 1, () => onLayout(moveRow(layout, r, 1)))}
                    {row.length > 1 && tool(`Левее: ${n}`, 'chevron-left', c === 0, () => onLayout(moveInRow(layout, r, c, -1)))}
                    {row.length > 1 && tool(`Правее: ${n}`, 'chevron-right', c === row.length - 1, () => onLayout(moveInRow(layout, r, c, 1)))}
                    {c > 0
                      ? tool(`В отдельную строку: ${n}`, 'rows-split', !canSplit(layout, r, c), () => onLayout(toggleJoin(layout, r, c, blocks)))
                      : s.kind === 'button' && r > 0 && tool(`В строку с предыдущей: ${n}`, 'rows-join', !canJoin(layout, r, c, blocks), () => onLayout(toggleJoin(layout, r, c, blocks)))}
                    <span className="grow" />
                    {s.kind === 'button' &&
                      (s.removable ? (
                        tool(`Скрыть: ${n}`, 'close', false, () => onLayout(hide(layout, id)))
                      ) : (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span tabIndex={0} role="img" aria-label={`Нужна для работы бота: ${n}`} className="focus-ring inline-flex size-7 items-center justify-center rounded-full text-text-secondary">
                              <Icon name="lock" size={16} />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>Нужна для работы бота</TooltipContent>
                        </Tooltip>
                      ))}
                  </div>
                </div>
              )
            })}
          </div>
        ))}
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
        Что за кнопка и когда она появляется, решает код бота; здесь — подпись, стиль, порядок и «за ненадобностью» скрытие. Подпись и стиль общие для всех
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
