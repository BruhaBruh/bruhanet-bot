export interface UserSummary {
  telegramId: number
  username: string
  name: string
  createdAt: string
  confirmed: number
  confirmedSum: number
  expireAt: string | null
  panelStatus: string | null
  blockedBotAt: string | null
  blockedAt: string | null
  tariffMonths: number | null
  extraDevices: number
  lastSeenAt: string | null
  broadcastsReceived: number
  trialUsedAt: string | null
  inTrial: boolean
}

export interface UserPage {
  items: UserSummary[]
  total: number
  page: number
  pages: number
}

export interface UserPromo {
  code: string
  kind: 'percent' | 'rub' | 'days'
  value: number
  activatedAt: string
}

export interface UserGrant {
  id: number
  days: number
  createdAt: string
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
  hasPhoto?: boolean
  promoCode?: string | null
  kind?: 'tariff' | 'devices' | 'gift'
  extraDevices?: number
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
    blocked: boolean
  }
  payments: Payment[]
  panel: { id: number | null; expireAt: string | null; status: string | null; error: 'panel_unavailable' | null }
  devices?: { base: number; extra: number; price: number }
  messages: DirectMessage[]
  promos: UserPromo[]
  grants: UserGrant[]
  referral: UserReferral
}

export interface UserReferral {
  invited: number
  paid: number
  referredBy: number | null
  referredByUsername: string
}

export interface TextVar {
  name: string
  description: string
  sample: string
  raw: boolean
  max: number
}

export interface TextItem {
  key: string
  group: string
  title: string
  kind: 'message' | 'button' | 'plain'
  default: string
  body: string
  overridden: boolean
  inactive?: boolean
  problem?: string
  vars: TextVar[]
  when: string
  buttons: string[]
  route?: string
  style: string
}

export interface Settings {
  prices: Record<string, number>
  phone: string
  bank: string
  comments: string[]
  supportUrl: string
  reminderTimes: string[]
  advanceLimit: number
  advanceHours: number
  reminderWeekDays: number
  reminderDayHours: number
  devicesBase: number
  devicePrice: number
  trialDays: number
  trialDevices: number
  referralDays: number
  vars: Record<string, string>
  usingDefaults?: boolean
  problems?: string[]
}

export type BroadcastStatus = 'draft' | 'scheduled' | 'sending' | 'sent' | 'cancelled'
export type Audience = 'all' | 'active' | 'no_subscription' | 'expiring' | 'trial'

export interface Broadcast {
  id: string
  status: BroadcastStatus
  text: string
  photo?: string
  audience: Audience
  audienceDays: number | null
  silent: boolean
  buttons: BroadcastButton[]
  scheduledAt: string | null
  total: number
  sent: number
  failed: number
  createdAt: string
  updatedAt: string
  finishedAt: string | null
}

export interface BroadcastButton {
  text: string
  url: string
  style: '' | 'primary' | 'success' | 'danger'
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
  validFrom: string | null // YYYY-MM-DD включительно, МСК
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

export type GiftStatus = 'pending' | 'ready' | 'redeemed' | 'cancelled'

// code пуст у pending: пока платёж не подтверждён, код не отдаётся.
export interface Gift {
  id: number
  code: string
  months: number
  status: GiftStatus
  buyerId: number | null
  buyerUsername: string
  paymentId: number | null
  byAdmin: boolean
  redeemedBy: number | null
  redeemedUsername: string
  redeemedAt: string | null
  createdAt: string
}
