export const MAX_DAYS = 365
const DAY_MS = 864e5

/** Срок после выдачи: от позднего из срока в панели и now, как на сервере (payflow.GrantDays). */
export function grantedExpire(expireAt: string | null, days: number, now = Date.now()): string {
  return new Date(Math.max(expireAt ? Date.parse(expireAt) : 0, now) + days * DAY_MS).toISOString()
}

/** Целое от 1 до 365 или undefined. */
export function parseDays(v: string): number | undefined {
  if (!/^\d{1,3}$/.test(v)) return undefined
  const n = Number(v)
  return n >= 1 && n <= MAX_DAYS ? n : undefined
}

