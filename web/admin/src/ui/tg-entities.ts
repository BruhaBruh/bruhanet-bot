import { cn } from '@/lib/utils'

// Сущности Telegram HTML внутри текста. Спойлер двумя способами: тег Bot API и класс из README DS.
export const tgEntities = cn(
  'wrap-anywhere',
  '[&_a]:text-link [&_a]:no-underline',
  '[&_code]:font-mono [&_code]:text-[14px] [&_code]:tracking-normal [&_code]:text-accent-strong',
  '[&_pre]:my-1 [&_pre]:rounded-xs [&_pre]:bg-fill-tertiary [&_pre]:px-2 [&_pre]:py-1.5 [&_pre]:font-mono [&_pre]:text-[14px] [&_pre]:tracking-normal [&_pre]:whitespace-pre-wrap',
  '[&_blockquote]:my-1 [&_blockquote]:rounded-xs [&_blockquote]:bg-accent-tint [&_blockquote]:px-2 [&_blockquote]:py-0.5 [&_blockquote]:shadow-[inset_3px_0_0_var(--accent)]',
  '[&_.tg-spoiler]:rounded-[4px] [&_.tg-spoiler]:bg-text-hint [&_.tg-spoiler]:text-transparent',
  '[&_tg-spoiler]:rounded-[4px] [&_tg-spoiler]:bg-text-hint [&_tg-spoiler]:text-transparent',
)
