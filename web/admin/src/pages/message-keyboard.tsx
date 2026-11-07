import { useState } from 'react'
import { Button, Card, Checkbox, DragHandle, Icon, Input, LayoutGrid, Select, Sheet, StyleSelect } from '@/ui'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/ui/primitives/tooltip'
import { MAX_ROWS } from '../lib/keyboard-layout'
import { fromLayout, MAX_CELLS, countCells, patchCell, removeCell, templatesLeft, tplName, toLayout, addButton, addTemplate } from '../lib/message-keyboard'
import type { CondRef, KbCatalogs, KbCellOut, MessageKeyboard } from '../types'

const BUTTON_LIMIT = 64
const cell = 'flex min-w-0 flex-col gap-1.5 rounded-md bg-fill-tertiary p-2'
const mono = 'caption-1 truncate font-mono text-text-secondary'

const actionTitle = (cat: KbCatalogs, id: string) => cat.actions.find((a) => a.id === id)?.title ?? id
const condTitle = (cat: KbCatalogs, id: string) => cat.conditions.find((c) => c.id === id)?.title ?? id
const conditionLine = (cat: KbCatalogs, cs: CondRef[]) => cs.map((c) => `${c.not ? 'НЕ ' : ''}${condTitle(cat, c.id)}`).join(' · ')

/** Модалка «Действие»: что делает кнопка. Привязанные к экрану действия (`bound`) здесь не выбираются. */
function ActionSheet({ c, cat, onApply, onClose }: { c: KbCellOut; cat: KbCatalogs; onApply(action: string): void; onClose(): void }) {
  const url = c.action.startsWith('url:')
  const [kind, setKind] = useState(url ? 'url' : c.action)
  const [link, setLink] = useState(url ? c.action.slice(4) : 'https://')
  const choices = cat.actions.filter((a) => a.kind !== 'url' && a.kind !== 'template' && (!a.bound || a.id === c.action))
  const action = kind === 'url' ? `url:${link.trim()}` : kind
  const guard = cat.actions.find((a) => a.id === kind)?.guard
  return (
    <Sheet
      title={`Действие: ${c.effective?.preview || c.label || c.id}`}
      onClose={onClose}
      footer={
        <div className="flex justify-end gap-3">
          <Button variant="plain" onClick={onClose}>
            Отмена
          </Button>
          <Button
            disabled={kind === 'url' && !/^https:\/\/\S+$/.test(link.trim())}
            onClick={() => {
              onApply(action)
              onClose()
            }}
          >
            Готово
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-3 px-4 pb-2">
        <label className="flex flex-col gap-1">
          <span className="admin-label text-text-secondary">Что делает кнопка</span>
          <Select
            aria-label="Действие кнопки"
            value={kind}
            onChange={setKind}
            options={[...choices.map((a) => ({ value: a.id, label: a.title })), { value: 'url', label: 'Ссылка (https)' }]}
          />
        </label>
        {kind === 'url' && <Input aria-label="Адрес ссылки" placeholder="https://" value={link} onChange={(e) => setLink(e.target.value)} hint="Только https://" />}
        {guard && <p className="footnote text-text-secondary">Показывается, только если выполнено условие «{condTitle(cat, guard)}»: его нельзя отключить.</p>}
      </div>
    </Sheet>
  )
}

/** Модалка «Условия»: кнопка показывается, когда выполнены все отмеченные (у каждого можно «НЕ»). */
function ConditionsSheet({ c, cat, onApply, onClose }: { c: KbCellOut; cat: KbCatalogs; onApply(cs: CondRef[]): void; onClose(): void }) {
  const [list, setList] = useState<CondRef[]>(c.conditions)
  const get = (id: string) => list.find((x) => x.id === id)
  const toggle = (id: string, on: boolean) => setList((l) => (on ? [...l.filter((x) => x.id !== id), { id }] : l.filter((x) => x.id !== id)))
  const flip = (id: string, not: boolean) => setList((l) => l.map((x) => (x.id === id ? (not ? { id, not: true } : { id }) : x)))
  return (
    <Sheet
      title="Когда показывать"
      onClose={onClose}
      footer={
        <div className="flex justify-end gap-3">
          <Button variant="plain" onClick={onClose}>
            Отмена
          </Button>
          <Button
            onClick={() => {
              onApply(cat.conditions.flatMap((k) => get(k.id) ?? []))
              onClose()
            }}
          >
            Готово
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-3 px-4 pb-2">
        <p className="footnote text-text-secondary">Без условий кнопка видна всегда. Несколько условий работают вместе (и).</p>
        {cat.conditions.map((k) => {
          const x = get(k.id)
          return (
            <div key={k.id} className="flex flex-col gap-1 rounded-md bg-fill-tertiary p-2">
              <Checkbox checked={!!x} label={k.title} onChange={(on) => toggle(k.id, on)} />
              <span className="footnote text-text-secondary">{k.description}</span>
              {x && <Checkbox checked={!!x.not} label="НЕ (наоборот)" onChange={(not) => flip(k.id, not)} />}
            </div>
          )
        })}
      </div>
    </Sheet>
  )
}

/** Текст кнопки, которой подписан каждый элемент шаблона: правится рядом с шаблоном, а не в клавиатуре. */
export interface TplText {
  label: string
  style: string
  /** Переменные текста через запятую, для подсказки. */
  vars: string
}

interface Props {
  kb: MessageKeyboard
  cat: KbCatalogs
  rows: KbCellOut[][]
  busy: boolean
  dirty: boolean
  saving: boolean
  tplText(key: string): TplText | undefined
  onTplText(key: string, change: { label?: string; style?: string }): void
  onRows(r: KbCellOut[][]): void
  onSave(): void
  onCancel(): void
  onReset(): void
}

export function MessageKeyboardCard({ kb, cat, rows, busy, dirty, saving, tplText, onTplText, onRows, onSave, onCancel, onReset }: Props) {
  const [modal, setModal] = useState<{ id: string; kind: 'action' | 'cond' }>()
  const cells = rows.flat()
  const byId = (id: string) => cells.find((c) => c.id === id)
  const tplOf = (c: KbCellOut) => cat.templates.find((t) => t.id === tplName(c))
  const name = (c: KbCellOut) => (c.template ? (tplOf(c)?.title ?? c.id) : c.label || c.effective?.preview || c.id)
  const blocks = cells.filter((c) => c.template).map((c) => c.id)
  const edit = (id: string, fn: (c: KbCellOut) => KbCellOut) => onRows(patchCell(rows, id, fn))
  const free = templatesLeft(rows, cat.templates.filter((t) => !kb.templates || kb.templates.includes(t.id)))
  const full = countCells(rows) >= MAX_CELLS || rows.length >= MAX_ROWS
  const m = modal && byId(modal.id)

  return (
    <Card title="Клавиатура" className="gap-3">
      {kb.inactive && (
        <p role="alert" className="admin-body rounded-lg bg-destructive-tint px-4 py-3 text-destructive-strong">
          Сохранённая клавиатура не применяется, бот показывает клавиатуру по умолчанию: {kb.problem}
        </p>
      )}
      <TooltipProvider delayDuration={200}>
        <LayoutGrid
          layout={toLayout(rows)}
          blocks={blocks}
          disabled={busy}
          name={(id) => {
            const c = byId(id)
            return c ? name(c) : id
          }}
          onChange={(l) => onRows(fromLayout(rows, l))}
          renderCell={(id) => {
            const c = byId(id)
            if (!c) return null
            const n = name(c)
            const trail = c.locked ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span tabIndex={0} role="img" aria-label={`Нужна для работы бота: ${n}`} className="focus-ring inline-flex size-6 items-center justify-center rounded-full text-text-secondary">
                    <Icon name="lock" size={14} />
                  </span>
                </TooltipTrigger>
                <TooltipContent>Нужна для работы бота</TooltipContent>
              </Tooltip>
            ) : (
              <button
                type="button"
                aria-label={`Убрать: ${n}`}
                disabled={busy}
                onClick={() => onRows(removeCell(rows, id))}
                className="focus-ring inline-flex size-6 flex-none cursor-pointer items-center justify-center rounded-sm text-text-secondary hover:bg-cell-highlight disabled:cursor-default disabled:opacity-40"
              >
                <Icon name="close" size={14} strokeWidth={2.4} />
              </button>
            )
            if (c.template) {
              const key = tplOf(c)?.key
              const t = key ? tplText(key) : undefined
              return (
                <div className={`${cell} border border-dashed border-separator`}>
                  <div className="flex items-center gap-1.5">
                    <DragHandle />
                    <code className={`${mono} grow`}>{c.action}</code>
                    {trail}
                  </div>
                  <span className="admin-body text-text-secondary">Шаблон: {n}</span>
                  {key && t && (
                    <>
                      <div className="flex items-start gap-2">
                        <Input
                          className="min-w-0 grow"
                          aria-label={`Текст кнопок шаблона ${n}`}
                          maxLength={BUTTON_LIMIT}
                          disabled={busy}
                          value={t.label}
                          onChange={(e) => onTplText(key, { label: e.target.value })}
                        />
                        <StyleSelect aria-label={`Стиль кнопок шаблона ${n}`} disabled={busy} value={t.style} onChange={(v) => onTplText(key, { style: v })} />
                      </div>
                      <span className="footnote text-text-secondary">
                        Подпись каждой кнопки блока (общая для всех экранов с ней, <code className="font-mono">{key}</code>). Переменные: {t.vars}.
                      </span>
                    </>
                  )}
                </div>
              )
            }
            const cond = conditionLine(cat, c.conditions)
            return (
              <div className={cell}>
                <div className="flex min-w-0 items-center gap-1.5">
                  <DragHandle />
                  <span className="flex min-w-0 grow flex-col">
                    <code className={mono}>{c.id}</code>
                    <code className={mono}>→ {actionTitle(cat, c.action)}</code>
                  </span>
                  {trail}
                </div>
                <div className="flex items-start gap-2">
                  <Input
                    className="min-w-0 grow"
                    placeholder={c.effective?.preview}
                    aria-label={`Текст кнопки ${n}`}
                    maxLength={BUTTON_LIMIT}
                    disabled={busy}
                    value={c.label ?? ''}
                    onChange={(e) => edit(id, (x) => ({ ...x, label: e.target.value }))}
                  />
                  <StyleSelect aria-label={`Стиль кнопки ${n}`} disabled={busy} value={c.style || c.effective?.style || ''} onChange={(v) => edit(id, (x) => ({ ...x, style: v }))} />
                </div>
                <div className="flex flex-wrap items-center gap-1.5">
                  <Button variant="gray" size="sm" disabled={busy || c.locked} aria-label={`Действие: ${n}`} onClick={() => setModal({ id, kind: 'action' })}>
                    Действие
                  </Button>
                  <Button variant="gray" size="sm" disabled={busy || c.locked} aria-label={`Условия: ${n}`} onClick={() => setModal({ id, kind: 'cond' })}>
                    {c.conditions.length ? `Условия: ${c.conditions.length}` : 'Условия'}
                  </Button>
                  {cond && <span className="footnote min-w-0 text-text-secondary">{cond}</span>}
                </div>
              </div>
            )
          }}
        />
      </TooltipProvider>

      <div className="flex flex-wrap items-center gap-2">
        <Button variant="gray" size="sm" disabled={busy || full} onClick={() => onRows(addButton(rows))}>
          + Кнопка
        </Button>
        {free.length > 0 && (
          <div>
            <Select
              aria-label="Добавить шаблон"
              className="h-8"
              placeholder="+ Шаблон"
              disabled={busy || full}
              value=""
              options={free.map((t) => ({ value: t.id, label: t.title }))}
              onChange={(id) => {
                const t = free.find((x) => x.id === id)
                if (t) onRows(addTemplate(rows, t))
              }}
            />
          </div>
        )}
        {full && <span className="footnote text-text-secondary">Не больше {MAX_CELLS} кнопок и {MAX_ROWS} рядов</span>}
      </div>

      <p className="footnote text-text-secondary">
        Кнопки принадлежат этому сообщению. Подпись пустая — берётся текст кнопки по умолчанию. Служебные кнопки (замок) убрать и перенастроить нельзя, но подпись, стиль и место менять можно.
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

      {m && modal?.kind === 'action' && <ActionSheet c={m} cat={cat} onApply={(a) => edit(m.id, (x) => ({ ...x, action: a }))} onClose={() => setModal(undefined)} />}
      {m && modal?.kind === 'cond' && <ConditionsSheet c={m} cat={cat} onApply={(cs) => edit(m.id, (x) => ({ ...x, conditions: cs }))} onClose={() => setModal(undefined)} />}
    </Card>
  )
}
