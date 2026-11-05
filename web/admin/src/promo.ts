import type { Promo } from './types'
import { formatRub, mskDate } from './format'

export type PromoKind = Promo['kind']

export const kindLabels: Record<PromoKind, string> = { percent: 'Скидка в %', rub: 'Скидка в рублях', days: 'Бонусные дни' }

// «−25%» / «−100 ₽» / «+7 дн.»
export function promoValue(p: Pick<Promo, 'kind' | 'value'>): string {
  if (p.kind === 'percent') return `−${p.value}%`
  if (p.kind === 'rub') return `−${formatRub(p.value)}`
  return `+${p.value}\u00a0дн.`
}

export type PromoStatus = 'active' | 'expired' | 'limit' | 'disabled'

// Порядок причин как у promo.Check на сервере; validUntil — включительная дата МСК.
export function promoStatus(p: Pick<Promo, 'disabled' | 'validUntil' | 'used' | 'maxActivations'>, now = new Date()): PromoStatus {
  if (p.disabled) return 'disabled'
  if (p.validUntil && p.validUntil < mskDate(now)) return 'expired'
  if (p.maxActivations != null && p.used >= p.maxActivations) return 'limit'
  return 'active'
}

// «@ann», «ID 7»: username есть не у всех
export const userLabel = (u: { username?: string | null; telegramId: number }) => (u.username ? `@${u.username}` : `ID ${u.telegramId}`)

// «до 31.08» / «—» (validUntil — YYYY-MM-DD)
export function promoUntil(validUntil: string | null): string {
  if (!validUntil) return '—'
  const [, m, d] = validUntil.split('-')
  return `до ${d}.${m}`
}

export interface PromoForm {
  code: string
  kind: PromoKind
  value: string
  maxActivations: string
  validUntil: string
  onlyNew: boolean
  user: string
}

const ranges: Record<PromoKind, [number, number, string]> = {
  percent: [1, 100, 'Скидка в процентах — от 1 до 100'],
  rub: [1, 1_000_000, 'Скидка в рублях — от 1 до 1 000 000'],
  days: [1, 3650, 'Бонусные дни — от 1 до 3650'],
}

// Клиентская проверка формы: текст первой ошибки или '' (сервер всё равно проверит).
export function validatePromo(f: PromoForm): string {
  const code = f.code.trim().toUpperCase()
  if (code && !/^[A-Z0-9_-]{3,32}$/.test(code)) return 'Код — 3–32 символа: латиница, цифры, _ и -'
  const [min, max, msg] = ranges[f.kind]
  const v = f.value.trim()
  if (!/^\d+$/.test(v) || +v < min || +v > max) return msg
  const m = f.maxActivations.trim()
  if (m && (!/^\d+$/.test(m) || +m < 1)) return 'Максимум активаций — не меньше 1'
  const u = f.user.trim()
  if (u && !/^(@\w+|\d+)$/.test(u)) return 'Юзер — @username или числовой ID'
  return ''
}

export function promoBody(f: PromoForm) {
  return {
    code: f.code.trim().toUpperCase(),
    kind: f.kind,
    value: Number(f.value.trim()),
    maxActivations: f.maxActivations.trim() ? Number(f.maxActivations.trim()) : null,
    validUntil: f.validUntil || null,
    onlyNew: f.onlyNew,
    user: f.user.trim(),
  }
}

const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'

// тот же алфавит и длина, что у серверного генератора
export function generateCode(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(8)), (b) => alphabet[b % alphabet.length]).join('')
}
