export interface UserSummary {
  telegramId: number
  username: string
  createdAt: string
  confirmed: number
  confirmedSum: number
}

export interface Payment {
  id: number | string
  telegramId: number
  username: string
  month: number
  price: number
  status: string
  prevExpireAt: string | null
  advanceGranted: boolean
  adminMessageId: number | null
  createdAt: string
  submittedAt: string | null
  resolvedAt: string | null
  promoId?: number | null
  basePrice?: number | null
}

export interface UserDetail {
  user: UserSummary
  stats: {
    confirmed: number
    confirmedSum: number
    unpaidAdvances: number
    lastConfirmedAt: string | null
    firstConfirmedAt: string | null
    hasHistory: boolean
  }
  payments: Payment[]
  panel: { expireAt: string | null; status: string | null; error: 'panel_unavailable' | null }
  messages: DirectMessage[]
}

export interface TextVar {
  name: string
  description: string
  sample: string
  raw: boolean
}

export interface TextItem {
  key: string
  group: string
  title: string
  kind: 'message' | 'button' | 'plain'
  default: string
  body: string
  overridden: boolean
  vars: TextVar[]
}

export interface Settings {
  prices: Record<string, number>
  phone: string
  bank: string
  comments: string[]
  supportUrl: string
  usingDefaults?: boolean
  problems?: string[]
}

export interface Broadcast {
  id: string
  text: string
  photo?: string
  total: number
  sent: number
  failed: number
  createdAt: string
  finishedAt: string | null
}

export interface DirectMessage {
  id: number
  telegramId: number
  text: string
  photo?: string
  status: 'pending' | 'sent' | 'failed'
  createdAt: string
}

export interface Promo {
  id: number
  code: string
  kind: 'percent' | 'rub' | 'days'
  value: number
  maxActivations: number | null
  used: number
  validUntil: string | null // YYYY-MM-DD включительно, МСК
  onlyNew: boolean
  telegramId: number | null
  username: string | null
  disabled: boolean
  createdAt: string
}

export interface PromoActivation {
  id: number
  promoId: number
  telegramId: number
  username: string
  paymentId: number | null
  createdAt: string
}
