import { useReducer } from 'react'
import { failedAvatars } from './tg-avatar-failed'
import { Avatar, type AvatarProps } from './avatar'

const avatarUrl = (tg: number | 'bot') => `/api/avatars/${tg}`

/** Настоящая аватарка из Telegram (через /api/avatars) с откатом на буквенную. `tg` — telegram id или 'bot'. */
export function TgAvatar({ tg, ...rest }: Omit<AvatarProps, 'src' | 'onImgError'> & { tg?: number | 'bot' }) {
  const [, rerender] = useReducer((x: number) => x + 1, 0)
  const key = String(tg)
  const src = tg && !failedAvatars.has(key) ? avatarUrl(tg) : undefined
  return (
    <Avatar
      {...rest}
      src={src}
      onImgError={() => {
        failedAvatars.add(key)
        rerender()
      }}
    />
  )
}
