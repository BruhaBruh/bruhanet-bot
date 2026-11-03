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
