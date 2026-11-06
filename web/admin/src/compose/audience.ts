import type { Audience } from '../types'
import { plural } from '../format'

export const AUDIENCES: { value: Audience; label: string }[] = [
  { value: 'all', label: 'Все пользователи' },
  { value: 'active', label: 'Активные подписчики' },
  { value: 'no_subscription', label: 'Без подписки' },
  { value: 'expiring', label: 'Подписка истекает через' },
]
export const DEFAULT_DAYS = '7'
export const MIN_DAYS = 1
export const MAX_DAYS = 60

export const daysValid = (v: string) => /^\d+$/.test(v) && Number(v) >= MIN_DAYS && Number(v) <= MAX_DAYS

// «Подписка истекает через 7 дн.»
export const audienceLabel = (a: Audience, days: string) =>
  a === 'expiring' ? `Подписка истекает через ${days} ${plural(Number(days), ['день', 'дня', 'дней'])}` : AUDIENCES.find((x) => x.value === a)!.label

