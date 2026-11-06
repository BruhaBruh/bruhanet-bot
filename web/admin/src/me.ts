import { createContext, useContext, useEffect, useState } from 'react'
import { get } from './api'

export interface BotInfo {
  username: string
  name: string
}

// Пустые поля — сервер не знает (Telegram недоступен): экраны прячут элемент, а не подставляют имя.
export interface Me {
  telegramId: number
  username: string
  name: string
  bot: BotInfo
}

export const NO_BOT: BotInfo = { username: '', name: '' }
export const NO_ME: Me = { telegramId: 0, username: '', name: '', bot: NO_BOT }

const MeContext = createContext<Me>(NO_ME)
export const MeProvider = MeContext.Provider

/** Кто вошёл и какой это бот; вне оболочки (страница входа) — пусто. */
export const useMe = () => useContext(MeContext)

const str = (v: unknown) => (typeof v === 'string' ? v : '')

export function parseBot(raw: Partial<BotInfo> | null | undefined): BotInfo {
  return { username: str(raw?.username), name: str(raw?.name) }
}

/** Грузит `/api/me` один раз; пока нет ответа или при сбое — пусто. */
export function useLoadMe(): Me {
  const [me, setMe] = useState(NO_ME)
  useEffect(() => {
    let stale = false
    get<Partial<Me>>('/api/me').then(
      (r) => !stale && setMe({ telegramId: Number(r.telegramId) || 0, username: str(r.username), name: str(r.name), bot: parseBot(r.bot) }),
      () => {},
    )
    return () => {
      stale = true
    }
  }, [])
  return me
}
