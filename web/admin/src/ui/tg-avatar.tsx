import { useReducer } from 'react'
import { failedAvatars } from './tg-avatar-failed'
import { Avatar, type AvatarProps } from './avatar'

const avatarUrl = (tg: number | 'bot') => `/api/avatars/${tg}`

/** Логотип бота: вместо буквы, пока Telegram не отдаёт фото бота (public/bot-avatar.png). */
export const BOT_FALLBACK = '/bot-avatar.png'

/** Настоящая аватарка из Telegram (через /api/avatars) с откатом на буквенную. `tg` — telegram id или 'bot'. */
export function TgAvatar({ tg, ...rest }: Omit<AvatarProps, 'src' | 'onImgError'> & { tg?: number | 'bot' }) {
  const [, rerender] = useReducer((x: number) => x + 1, 0)
  const key = String(tg)
  const fallbackKey = `${key}:fallback`
  let src: string | undefined
  if (tg && !failedAvatars.has(key)) src = avatarUrl(tg)
  else if (tg === 'bot' && !failedAvatars.has(fallbackKey)) src = BOT_FALLBACK
  return (
    <Avatar
      {...rest}
      src={src}
      onImgError={() => {
        // сначала отказ фото Telegram (у бота дальше логотип), потом отказ и логотипа — буква
        failedAvatars.add(src === BOT_FALLBACK ? fallbackKey : key)
        rerender()
      }}
    />
  )
}
