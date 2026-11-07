import type { ReactNode } from 'react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './primitives/tooltip'

/**
 * Подсказка вместо атрибута `title`: наш Tooltip появляется и при наведении, и при фокусе с клавиатуры, не зависит от
 * браузера и темы. child — один элемент (кнопка, span), он получает подсказку как есть; доступное имя (aria-label)
 * остаётся за самим элементом. Пустой label — подсказки нет.
 */
export function Tip({ label, children }: { label?: ReactNode; children: ReactNode }) {
  if (!label) return <>{children}</>
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>{children}</TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
