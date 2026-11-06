import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/ui/primitives/tooltip'
import type { TextVar } from '../types'
import { varLabel } from './serialize'

// Группа чипов-переменных: клик вставляет {{.name}} в редактор; описание — подсказка при наведении и фокусе.
export function VarChips({ title, vars, disabled, onInsert }: { title: string; vars: TextVar[]; disabled?: boolean; onInsert(name: string): void }) {
  if (!vars.length) return null
  return (
    <TooltipProvider delayDuration={200}>
      <div className="flex flex-col gap-2">
        <span className="admin-label text-text-secondary">{title}</span>
        <div className="flex flex-wrap gap-1.5">
          {vars.map((v) => (
            <Tooltip key={v.name}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  disabled={disabled}
                  className="mono focus-ring inline-flex h-7 max-w-full cursor-pointer items-center rounded-full bg-accent-tint px-2.5 font-semibold text-accent-strong disabled:cursor-default disabled:opacity-40"
                  onClick={() => onInsert(v.name)}
                >
                  <span className="truncate">{varLabel(v.name)}</span>
                </button>
              </TooltipTrigger>
              <TooltipContent className="max-w-72">
                {v.description}
                {v.sample && <span className="block opacity-70">Например: {v.sample}</span>}
              </TooltipContent>
            </Tooltip>
          ))}
        </div>
      </div>
    </TooltipProvider>
  )
}
