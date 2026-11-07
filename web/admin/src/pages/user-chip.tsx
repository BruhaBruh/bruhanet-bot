import { TgAvatar } from '@/ui'

/** Аватарка (Telegram, откат на букву) и подпись юзера в одну строку; prefix — слово перед ним («куплен»). */
export function UserChip({ id, label, prefix, size = 20 }: { id: number; label: string; prefix?: string; size?: number }) {
  return (
    <span className="inline-flex max-w-full items-center gap-1.5 align-middle">
      {prefix && <span className="flex-none">{`${prefix} `}</span>}
      <TgAvatar tg={id} name={label} id={id} size={size} />
      <span className="min-w-0 truncate">{label}</span>
    </span>
  )
}
