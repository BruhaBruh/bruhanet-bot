import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Icon } from './icon'

export type FormatKey = 'bold' | 'italic' | 'underline' | 'strike' | 'code' | 'pre' | 'spoiler' | 'quote' | 'link'

const CODE = 'font-mono text-[12px] font-semibold'

const FORMATS: { key: FormatKey; label?: string; title: string; className?: string }[] = [
  { key: 'bold', label: 'B', title: 'Жирный', className: 'font-bold' },
  // Georgia — как в bundle.css DS: у системного sans курсив не отличить от прямого на кнопке.
  { key: 'italic', label: 'I', title: 'Курсив', className: "italic [font-family:Georgia,'Times_New_Roman',serif]" },
  { key: 'underline', label: 'U', title: 'Подчёркнутый', className: 'underline' },
  { key: 'strike', label: 'S', title: 'Зачёркнутый', className: 'line-through' },
  { key: 'code', label: '</>', title: 'Моноширинный в строке', className: CODE },
  { key: 'pre', label: '{ }', title: 'Блок кода', className: CODE },
  { key: 'spoiler', label: '▒', title: 'Спойлер' },
  { key: 'quote', label: '❝', title: 'Цитата' },
  { key: 'link', title: 'Ссылка' },
]

export interface FormatBarProps {
  active?: FormatKey[]
  formats?: FormatKey[]
  onFormat?: (key: FormatKey) => void
  right?: ReactNode
  className?: string
}

export function FormatBar({ active = [], formats, onFormat, right, className }: FormatBarProps) {
  return (
    <div
      role="toolbar"
      aria-label="Форматирование"
      className={cn(
        'flex items-center gap-0.5 rounded-md bg-surface p-1 shadow-[var(--shadow-card),inset_0_0_0_0.5px_var(--separator)]',
        className,
      )}
    >
      {FORMATS.filter((f) => !formats || formats.includes(f.key)).map((f) => {
        const on = active.includes(f.key)
        return (
          <button
            key={f.key}
            type="button"
            title={f.title}
            aria-label={f.title}
            aria-pressed={on}
            className={cn(
              'focus-ring inline-flex size-8 cursor-pointer items-center justify-center rounded-xs text-[16px] text-text hover:bg-fill-tertiary',
              on && 'bg-accent-tint text-accent-strong hover:bg-accent-tint',
              f.className,
            )}
            onClick={() => onFormat?.(f.key)}
          >
            {f.label ?? <Icon name="link" size={18} strokeWidth={2.2} />}
          </button>
        )
      })}
      {right && <span className="caption-1 ml-auto px-2 text-text-secondary">{right}</span>}
    </div>
  )
}
