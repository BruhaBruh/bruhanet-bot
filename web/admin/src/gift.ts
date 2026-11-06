import type { Gift } from './types'

/** Ссылка подарка; пусто, если не знаем бота или код ещё не открыт (pending). */
export const giftLink = (botUsername: string, g: Pick<Gift, 'code'>) =>
  botUsername && g.code ? `https://t.me/${botUsername}?start=gift_${g.code}` : ''
